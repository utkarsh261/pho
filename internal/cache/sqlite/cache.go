package sqlitecache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/utk/git-term/internal/domain"
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
		"PRAGMA journal_mode=DELETE;",
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

func toUnixMillis(t time.Time) int64 {
	return t.UnixMilli()
}

func fromUnixMillis(v int64) time.Time {
	return time.UnixMilli(v).UTC()
}
