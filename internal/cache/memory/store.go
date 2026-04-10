package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/utk/git-term/internal/domain"
)

// JSONStore adapts the generic in-memory cache to the common cache Store
// contract using JSON serialization.
type JSONStore struct {
	inner *Cache[[]byte, domain.CacheMeta]
}

func NewJSONStore(maxBytes int) *JSONStore {
	return &JSONStore{
		inner: New[[]byte, domain.CacheMeta](maxBytes),
	}
}

// Get retrieves and decodes a value by key.
func (s *JSONStore) Get(_ context.Context, key string, dest any) (domain.CacheMeta, bool, error) {
	raw, metaWrap, ok := s.inner.Get(key)
	if !ok {
		return domain.CacheMeta{}, false, nil
	}
	meta := metaWrap.Data
	if dest != nil {
		if err := json.Unmarshal(raw, dest); err != nil {
			s.inner.Delete(key)
			return domain.CacheMeta{}, false, nil
		}
	}
	return meta, true, nil
}

// Put encodes and stores a value by key.
func (s *JSONStore) Put(_ context.Context, key string, value any, meta domain.CacheMeta) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("memory cache marshal %q: %w", key, err)
	}
	meta.Key = key
	meta.SizeBytes = len(raw)
	s.inner.Put(key, raw, Meta[domain.CacheMeta]{
		Group: groupForKind(meta.Kind),
		Bytes: len(raw),
		Data:  meta,
	})
	return nil
}

// Delete removes key from the in-memory store.
func (s *JSONStore) Delete(_ context.Context, key string) error {
	s.inner.Delete(key)
	return nil
}

func groupForKind(kind string) Group {
	switch kind {
	case "preview":
		return GroupPreview
	case "search_pr_index":
		return GroupPRIndex
	case "dashboard_recent":
		return GroupRecent
	case "dashboard_prs", "dashboard_involving":
		return GroupDashboard
	case "discovery":
		return GroupDiscovery
	case "search_repo_index":
		return GroupRepoIndex
	default:
		return GroupDashboard
	}
}
