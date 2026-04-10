// Package memory provides an in-process byte-bounded cache with priority-aware
// LRU eviction.
package memory

import (
	"container/list"
	"sync"
)

// Group identifies the cache class an item belongs to.
//
// Eviction order is defined by priority buckets, not by the enum order alone.
// Discovery and repo-index entries share the same lowest-priority bucket.
type Group uint8

const (
	GroupPreview Group = iota
	GroupPRIndex
	GroupRecent
	GroupDashboard
	GroupDiscovery
	GroupRepoIndex
)

// Meta stores cache metadata alongside the typed value.
type Meta[M any] struct {
	Group Group
	Bytes int
	Data  M
}

// Cache stores typed values with associated metadata and evicts entries using
// a byte budget plus priority-aware LRU rules.
type Cache[V any, M any] struct {
	mu       sync.Mutex
	maxBytes int
	used     int
	entries  map[string]*entry[V, M]
	buckets  [5]*list.List
}

type entry[V any, M any] struct {
	key   string
	value V
	meta  Meta[M]
	elem  *list.Element
}

// New creates a cache with the given maximum byte budget. A non-positive
// budget disables storage for byteful entries.
func New[V any, M any](maxBytes int) *Cache[V, M] {
	c := &Cache[V, M]{
		maxBytes: maxBytes,
		entries:  make(map[string]*entry[V, M]),
	}
	for i := range c.buckets {
		c.buckets[i] = list.New()
	}
	return c
}

func (c *Cache[V, M]) Get(key string) (V, Meta[M], bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var zeroV V
	var zeroM Meta[M]

	ent, ok := c.entries[key]
	if !ok {
		return zeroV, zeroM, false
	}

	c.buckets[priorityBucket(ent.meta.Group)].MoveToFront(ent.elem)
	return ent.value, ent.meta, true
}

// Put stores or replaces a value for key.
func (c *Cache[V, M]) Put(key string, value V, meta Meta[M]) {
	c.mu.Lock()
	defer c.mu.Unlock()

	bytes := meta.Bytes
	if bytes < 0 {
		bytes = 0
	}
	meta.Bytes = bytes

	if old, ok := c.entries[key]; ok {
		c.removeEntry(old)
	}

	if c.maxBytes <= 0 && bytes > 0 {
		return
	}
	if c.maxBytes > 0 && bytes > c.maxBytes {
		return
	}

	ent := &entry[V, M]{
		key:   key,
		value: value,
		meta:  meta,
	}
	bucket := c.buckets[priorityBucket(meta.Group)]
	ent.elem = bucket.PushFront(ent)
	c.entries[key] = ent
	c.used += bytes

	c.evictIfNeeded()
}

// Delete removes key from the cache if present.
func (c *Cache[V, M]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ent, ok := c.entries[key]; ok {
		c.removeEntry(ent)
	}
}

func (c *Cache[V, M]) evictIfNeeded() {
	if c.maxBytes < 0 {
		return
	}
	for c.used > c.maxBytes {
		victim := c.pickVictim()
		if victim == nil {
			return
		}
		c.removeEntry(victim)
	}
}

func (c *Cache[V, M]) pickVictim() *entry[V, M] {
	for _, bucket := range c.buckets {
		if back := bucket.Back(); back != nil {
			return back.Value.(*entry[V, M])
		}
	}
	return nil
}

func (c *Cache[V, M]) removeEntry(ent *entry[V, M]) {
	if ent == nil {
		return
	}
	bucket := c.buckets[priorityBucket(ent.meta.Group)]
	if ent.elem != nil {
		bucket.Remove(ent.elem)
		ent.elem = nil
	}
	delete(c.entries, ent.key)
	c.used -= ent.meta.Bytes
	if c.used < 0 {
		c.used = 0
	}
}

func priorityBucket(group Group) int {
	switch group {
	case GroupPreview:
		return 0
	case GroupPRIndex:
		return 1
	case GroupRecent:
		return 2
	case GroupDashboard:
		return 3
	case GroupDiscovery, GroupRepoIndex:
		return 4
	default:
		return 4
	}
}
