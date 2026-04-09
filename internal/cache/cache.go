package cache

import (
	"context"

	"github.com/utk/git-term/internal/domain"
)

// Store is the common cache contract used by the two-tier cache coordinator.
type Store interface {
	Get(ctx context.Context, key string, dest any) (domain.CacheMeta, bool, error)
	Put(ctx context.Context, key string, value any, meta domain.CacheMeta) error
	Delete(ctx context.Context, key string) error
}
