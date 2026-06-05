// Package cache provides a tiny concurrency-safe value cache plus a background
// poller. The design goal is reliability for a wall display: rendering only
// ever reads the last good value, and a failed upstream fetch never overwrites
// good data — the dashboard degrades to "stale with a timestamp" instead of
// going blank.
package cache

import (
	"context"
	"log"
	"sync"
	"time"
)

// Cache holds the most recent successfully-fetched value of type T.
type Cache[T any] struct {
	mu        sync.RWMutex
	data      T
	updatedAt time.Time
	ok        bool // has at least one fetch ever succeeded?
}

// New returns an empty cache.
func New[T any]() *Cache[T] { return &Cache[T]{} }

// Set stores a freshly fetched value and stamps the update time.
func (c *Cache[T]) Set(v T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data, c.updatedAt, c.ok = v, time.Now(), true
}

// Snapshot returns the last good value, when it was fetched, and whether any
// fetch has ever succeeded. Callers decide what counts as "stale".
func (c *Cache[T]) Snapshot() (value T, updatedAt time.Time, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data, c.updatedAt, c.ok
}

// Poll fetches immediately, then on every tick of the given interval, writing
// successes into the cache. On error it logs and keeps the previous value.
// It returns when ctx is cancelled (graceful shutdown).
func Poll[T any](ctx context.Context, c *Cache[T], name string, every time.Duration, fetch func(context.Context) (T, error)) {
	run := func() {
		fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		v, err := fetch(fctx)
		if err != nil {
			log.Printf("[%s] fetch failed, keeping stale data: %v", name, err)
			return
		}
		c.Set(v)
		log.Printf("[%s] refreshed", name)
	}

	run() // populate at startup rather than waiting one interval
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
