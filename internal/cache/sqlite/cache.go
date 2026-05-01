package sqlitecache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/utkarsh261/pho/internal/domain"
)

const (
	defaultVersion  = 1
	defaultEncoding = "json"
)

type Cache struct {
	db      *sql.DB
	version int
}

func New(dbPath string, version int) (*Cache, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("sqlite cache path is empty")
	}
	if version <= 0 {
		version = defaultVersion
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite cache dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite cache: %w", err)
	}
	// Single connection eliminates SQLITE_BUSY: all reads and writes are
	// serialized through one connection, so there is never a second writer
	// waiting on the lock.
	db.SetMaxOpenConns(1)

	c := &Cache{
		db:      db,
		version: version,
	}
	if err := c.bootstrap(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return c, nil
}

func (c *Cache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func (c *Cache) bootstrap(ctx context.Context) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, q := range pragmas {
		if _, err := c.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("sqlite pragma %q: %w", q, err)
		}
	}

	schema := []string{
		`CREATE TABLE IF NOT EXISTS cache_entries (
			key TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			version INTEGER NOT NULL,
			host TEXT NOT NULL,
			repo TEXT,
			pr_number INTEGER,
			fetched_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			etag TEXT,
			last_modified TEXT,
			size_bytes INTEGER NOT NULL,
			encoding TEXT NOT NULL,
			payload BLOB NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_cache_kind_repo ON cache_entries(kind, host, repo);`,
		`CREATE INDEX IF NOT EXISTS idx_cache_repo_pr ON cache_entries(host, repo, pr_number);`,
		`CREATE TABLE IF NOT EXISTS cache_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS viewed_history (
			host TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			summary_json TEXT NOT NULL,
			last_viewed_at INTEGER NOT NULL,
			PRIMARY KEY (host, repo, pr_number)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_viewed_history_repo ON viewed_history(host, repo);`,
	}
	for _, q := range schema {
		if _, err := c.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("sqlite schema: %w", err)
		}
	}
	return nil
}

// If the key is not found, found is false.
// If the row exists but has an incompatible version or corrupt payload, the row
// is deleted and treated as a miss.
func (c *Cache) Get(ctx context.Context, key string, dest any) (meta domain.CacheMeta, found bool, err error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT
			kind, version, host, repo, pr_number, fetched_at, expires_at,
			etag, last_modified, size_bytes, encoding, payload
		FROM cache_entries
		WHERE key = ?
	`, key)

	var (
		repo         sql.NullString
		prNum        sql.NullInt64
		etag         sql.NullString
		lastModified sql.NullString
		payload      []byte
		fetchedAt    int64
		expiresAt    int64
	)
	meta = domain.CacheMeta{Key: key}
	err = row.Scan(
		&meta.Kind,
		&meta.Version,
		&meta.Host,
		&repo,
		&prNum,
		&fetchedAt,
		&expiresAt,
		&etag,
		&lastModified,
		&meta.SizeBytes,
		&meta.Encoding,
		&payload,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.CacheMeta{}, false, nil
		}
		return domain.CacheMeta{}, false, fmt.Errorf("sqlite cache get %q: %w", key, err)
	}

	meta.Repo = repo.String
	if prNum.Valid {
		v := int(prNum.Int64)
		meta.PRNumber = &v
	}
	meta.ETag = etag.String
	meta.LastModified = lastModified.String
	meta.FetchedAt = fromUnixMillis(fetchedAt)
	meta.ExpiresAt = fromUnixMillis(expiresAt)

	if meta.Version != c.version {
		_ = c.Delete(ctx, key)
		return domain.CacheMeta{}, false, nil
	}
	if dest != nil {
		if err := json.Unmarshal(payload, dest); err != nil {
			_ = c.Delete(ctx, key)
			return domain.CacheMeta{}, false, nil
		}
	}
	return meta, true, nil
}

func (c *Cache) Put(ctx context.Context, key string, value any, meta domain.CacheMeta) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("sqlite cache marshal %q: %w", key, err)
	}

	now := time.Now()
	if meta.Key == "" {
		meta.Key = key
	}
	if meta.Version <= 0 {
		meta.Version = c.version
	}
	if meta.Encoding == "" {
		meta.Encoding = defaultEncoding
	}
	if meta.FetchedAt.IsZero() {
		meta.FetchedAt = now
	}
	if meta.ExpiresAt.IsZero() {
		meta.ExpiresAt = meta.FetchedAt
	}
	meta.SizeBytes = len(payload)

	var prNum any
	if meta.PRNumber != nil {
		prNum = *meta.PRNumber
	}

	_, err = c.db.ExecContext(ctx, `
		INSERT INTO cache_entries(
			key, kind, version, host, repo, pr_number, fetched_at, expires_at,
			etag, last_modified, size_bytes, encoding, payload
		)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			kind=excluded.kind,
			version=excluded.version,
			host=excluded.host,
			repo=excluded.repo,
			pr_number=excluded.pr_number,
			fetched_at=excluded.fetched_at,
			expires_at=excluded.expires_at,
			etag=excluded.etag,
			last_modified=excluded.last_modified,
			size_bytes=excluded.size_bytes,
			encoding=excluded.encoding,
			payload=excluded.payload
	`, key, meta.Kind, meta.Version, meta.Host, meta.Repo, prNum,
		toUnixMillis(meta.FetchedAt), toUnixMillis(meta.ExpiresAt),
		meta.ETag, meta.LastModified, meta.SizeBytes, meta.Encoding, payload,
	)
	if err != nil {
		return fmt.Errorf("sqlite cache put %q: %w", key, err)
	}
	return nil
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM cache_entries WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("sqlite cache delete %q: %w", key, err)
	}
	return nil
}

// DeleteByRepo removes all cache entries for a given host and repo.
func (c *Cache) DeleteByRepo(ctx context.Context, host, repo string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM cache_entries WHERE host = ? AND repo = ?`, host, repo)
	if err != nil {
		return fmt.Errorf("sqlite cache delete by repo %s/%s: %w", host, repo, err)
	}
	return nil
}

// LoadViewedHistory returns the persisted viewed PR records for a repository,
// ordered by most recently viewed first.
func (c *Cache) LoadViewedHistory(ctx context.Context, repo domain.Repository) ([]domain.ViewedPRRecord, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT pr_number, summary_json, last_viewed_at
		FROM viewed_history
		WHERE host = ? AND repo = ?
		ORDER BY last_viewed_at DESC
	`, repo.Host, repo.FullName)
	if err != nil {
		return nil, fmt.Errorf("sqlite load viewed history %q: %w", repo.FullName, err)
	}
	defer rows.Close()

	var out []domain.ViewedPRRecord
	for rows.Next() {
		var (
			prNumber     int
			summaryJSON  string
			lastViewedAt int64
		)
		if err := rows.Scan(&prNumber, &summaryJSON, &lastViewedAt); err != nil {
			return nil, fmt.Errorf("sqlite scan viewed history %q: %w", repo.FullName, err)
		}
		var summary domain.PullRequestSummary
		if err := json.Unmarshal([]byte(summaryJSON), &summary); err != nil {
			// Skip corrupt rows rather than failing the entire load.
			continue
		}
		out = append(out, domain.ViewedPRRecord{
			Repo:         repo.FullName,
			Number:       prNumber,
			Summary:      summary,
			LastViewedAt: fromUnixMillis(lastViewedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite viewed history rows %q: %w", repo.FullName, err)
	}
	return out, nil
}

// SaveViewedHistory atomically replaces the viewed history for a repository
// with the provided records. Records for other repositories are untouched.
func (c *Cache) SaveViewedHistory(ctx context.Context, repo domain.Repository, records []domain.ViewedPRRecord) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite save viewed history begin %q: %w", repo.FullName, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM viewed_history WHERE host = ? AND repo = ?`,
		repo.Host, repo.FullName,
	); err != nil {
		return fmt.Errorf("sqlite save viewed history delete %q: %w", repo.FullName, err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO viewed_history(host, repo, pr_number, summary_json, last_viewed_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("sqlite save viewed history prepare %q: %w", repo.FullName, err)
	}
	defer stmt.Close()

	for _, r := range records {
		if !sameRepo(r.Repo, repo.FullName) {
			continue
		}
		summaryJSON, err := json.Marshal(r.Summary)
		if err != nil {
			return fmt.Errorf("sqlite save viewed history marshal %s#%d: %w", r.Repo, r.Number, err)
		}
		if _, err := stmt.ExecContext(ctx,
			repo.Host, repo.FullName, r.Number, string(summaryJSON), toUnixMillis(r.LastViewedAt),
		); err != nil {
			return fmt.Errorf("sqlite save viewed history insert %s#%d: %w", r.Repo, r.Number, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite save viewed history commit %q: %w", repo.FullName, err)
	}
	return nil
}

func sameRepo(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func toUnixMillis(t time.Time) int64 {
	return t.UnixMilli()
}

func fromUnixMillis(v int64) time.Time {
	return time.UnixMilli(v).UTC()
}
