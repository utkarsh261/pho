package pr

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/utkarsh261/pho/internal/diff/anchor"
	"github.com/utkarsh261/pho/internal/diff/model"
	"github.com/utkarsh261/pho/internal/diff/parse"
	"github.com/utkarsh261/pho/internal/domain"
)

// LegacyDiffKeyPrefixes are cache keys in the old format that stored the
// parsed model as JSON. pho no longer reads them; startup deletes them.
var LegacyDiffKeyPrefixes = []string{"diff:v1:", "commitdiff:v1:"}

// cachedDiffFormat identifies cachedDiff.Gz as a gzip-compressed unified diff.
const cachedDiffFormat = "unified+gzip"

// cacheWriteTimeout bounds a background diff cache write.
const cacheWriteTimeout = 10 * time.Second

// cachedDiff is what the diff cache stores: the unified diff text (as
// fetched, or rebuilt from per-file patches), compressed. It is a fraction
// of the parsed model's size and re-parses faster than JSON decodes.
type cachedDiff struct {
	Format       string `json:"format"`
	Gz           []byte `json:"gz"`
	PartialFiles bool   `json:"partial_files,omitempty"`
}

func encodeCachedDiff(raw string, partial bool) (cachedDiff, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := io.Copy(zw, strings.NewReader(raw)); err != nil {
		return cachedDiff{}, err
	}
	if err := zw.Close(); err != nil {
		return cachedDiff{}, err
	}
	return cachedDiff{Format: cachedDiffFormat, Gz: buf.Bytes(), PartialFiles: partial}, nil
}

func (c cachedDiff) decode() (string, error) {
	if c.Format != cachedDiffFormat {
		return "", fmt.Errorf("unknown cached diff format %q", c.Format)
	}
	zr, err := gzip.NewReader(bytes.NewReader(c.Gz))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if _, err := io.Copy(&b, zr); err != nil {
		return "", err
	}
	return b.String(), nil
}

// buildDiffModel turns raw diff text into the model the UI shows. Fresh
// fetches and cache hits both go through it, so a cached diff can never be
// numbered or anchored differently from a fresh one.
func buildDiffModel(raw string, partial bool, repo domain.Repository, number int, headSHA string) (*model.DiffModel, error) {
	dm, err := parse.Parse(raw)
	if err != nil {
		return nil, err
	}
	dm.PartialFiles = partial
	// HeadSHA comes from the GraphQL result, not from the diff's index lines.
	dm.HeadSHA = headSHA
	dm.Repo = repoFullName(repo)
	dm.PRNumber = number
	anchor.Generate(dm, headSHA)
	// Precompute StartRow for file-level virtualization.
	cumulative := 0
	for i := range dm.Files {
		dm.Files[i].StartRow = cumulative
		cumulative += dm.Files[i].DisplayRows
	}
	return dm, nil
}

// readCachedDiff loads and builds a cached diff. An entry that can't be
// decoded is deleted and reported as a miss, so it is refetched.
func (s *PRService) readCachedDiff(ctx context.Context, key string, build func(raw string, partial bool) (*model.DiffModel, error)) (model.DiffModel, bool) {
	var entry cachedDiff
	_, _, found, _ := s.Cache.StaleWhileRevalidate(ctx, key, &entry, nil)
	if !found {
		return model.DiffModel{}, false
	}
	raw, err := entry.decode()
	var dm *model.DiffModel
	if err == nil {
		dm, err = build(raw, entry.PartialFiles)
	}
	if err != nil {
		s.logWarn("unreadable cached diff, refetching", "key", key, "err", err)
		_ = s.Cache.Delete(ctx, key)
		return model.DiffModel{}, false
	}
	return *dm, true
}

// writeCachedDiff compresses and stores raw in the background so the diff
// shows without waiting on the cache. after, if set, runs once the write
// succeeds (used to drop the PR's older diffs). Failures are only logged.
func (s *PRService) writeCachedDiff(key, raw string, partial bool, meta domain.CacheMeta, after func(context.Context) error) {
	s.spawnBackground(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cacheWriteTimeout)
		defer cancel()
		entry, err := encodeCachedDiff(raw, partial)
		if err != nil {
			s.logWarn("diff cache encode error", "key", key, "err", err)
			return
		}
		if err := s.Cache.Write(ctx, key, entry, meta); err != nil {
			s.logWarn("diff cache write error", "key", key, "err", err)
			return
		}
		if after != nil {
			if err := after(ctx); err != nil {
				s.logWarn("diff cache prune error", "key", key, "err", err)
			}
		}
	})
}
