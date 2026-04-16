package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/utkarsh261/pho/internal/domain"
	gitlog "github.com/utkarsh261/pho/internal/log"
)

// Coordinator wires L1 and L2 stores together and exposes a stale-while-
// revalidate read path.
type Coordinator struct {
	L1  Store
	L2  Store
	Now func() time.Time
	log *gitlog.Logger
}

// NewCoordinator constructs a two-tier cache coordinator.
func NewCoordinator(l1 Store, l2 Store, logger *gitlog.Logger) *Coordinator {
	if logger == nil {
		logger = gitlog.NewNop()
	}
	return &Coordinator{
		L1:  l1,
		L2:  l2,
		Now: time.Now,
		log: logger,
	}
}

// StaleWhileRevalidate reads the key from L1 then L2 with L1 promotion.
// If stale data is returned, scheduleRefresh is called with the key.
func (c *Coordinator) StaleWhileRevalidate(
	ctx context.Context,
	key string,
	dest any,
	scheduleRefresh func(key string),
) (meta domain.CacheMeta, freshness domain.Freshness, found bool, err error) {
	meta, found, err = c.L1.Get(ctx, key, dest)
	if err != nil {
		c.log.Warn("cache error", gitlog.FieldCacheKey, key, "err", err)
		return domain.CacheMeta{}, domain.FreshnessErrorStale, false, fmt.Errorf("l1 get %q: %w", key, err)
	}
	if found {
		c.log.Debug("cache swr", gitlog.FieldCacheKey, key, "found", true, "stale", false, "revalidating", scheduleRefresh != nil)
		freshness = c.freshness(meta)
		if freshness != domain.FreshnessFresh && scheduleRefresh != nil {
			scheduleRefresh(key)
		}
		return meta, freshness, true, nil
	}

	meta, found, err = c.L2.Get(ctx, key, dest)
	if err != nil {
		c.log.Warn("cache error", gitlog.FieldCacheKey, key, "err", err)
		return domain.CacheMeta{}, domain.FreshnessErrorStale, false, fmt.Errorf("l2 get %q: %w", key, err)
	}
	if !found {
		c.log.Debug("cache swr", gitlog.FieldCacheKey, key, "found", false, "stale", false, "revalidating", scheduleRefresh != nil)
		return domain.CacheMeta{}, domain.FreshnessStale, false, nil
	}

	stale := c.freshness(meta) != domain.FreshnessFresh
	c.log.Debug("cache swr", gitlog.FieldCacheKey, key, "found", true, "stale", stale, "revalidating", scheduleRefresh != nil)

	// L2 hit: promote to L1. Promotion failure should not discard useful data.
	if putErr := c.L1.Put(ctx, key, dest, meta); putErr != nil {
		c.log.Warn("cache error", gitlog.FieldCacheKey, key, "err", putErr)
		err = errors.Join(err, fmt.Errorf("l1 promote %q: %w", key, putErr))
	}
	freshness = c.freshness(meta)
	if freshness != domain.FreshnessFresh && scheduleRefresh != nil {
		scheduleRefresh(key)
	}
	return meta, freshness, true, err
}

// Write stores the same value in both tiers.
// L2 is attempted first. L1 write still proceeds if L2 write fails.
func (c *Coordinator) Write(ctx context.Context, key string, value any, meta domain.CacheMeta) error {
	var errs []error

	if err := c.L2.Put(ctx, key, value, meta); err != nil {
		errs = append(errs, fmt.Errorf("l2 put %q: %w", key, err))
	}
	if err := c.L1.Put(ctx, key, value, meta); err != nil {
		errs = append(errs, fmt.Errorf("l1 put %q: %w", key, err))
	}
	return errors.Join(errs...)
}

// Delete removes the key from both tiers.
func (c *Coordinator) Delete(ctx context.Context, key string) error {
	var errs []error
	if err := c.L1.Delete(ctx, key); err != nil {
		errs = append(errs, fmt.Errorf("l1 delete %q: %w", key, err))
	}
	if err := c.L2.Delete(ctx, key); err != nil {
		errs = append(errs, fmt.Errorf("l2 delete %q: %w", key, err))
	}
	return errors.Join(errs...)
}

func (c *Coordinator) freshness(meta domain.CacheMeta) domain.Freshness {
	now := c.Now()
	if meta.ExpiresAt.IsZero() || now.After(meta.ExpiresAt) {
		return domain.FreshnessStale
	}
	return domain.FreshnessFresh
}
