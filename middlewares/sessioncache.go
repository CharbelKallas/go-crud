package middlewares

import (
	"context"
	"sync"
	"time"
)

// maxCachedSessions bounds the cache; when full it is emptied.
const maxCachedSessions = 10_000

// CachedSessions wraps a SessionStore and remembers each user's session
// version for ttl, so authenticated requests do not each cost a database
// round trip. A revocation therefore takes effect within ttl. Errors and
// unknown users are not cached.
type CachedSessions struct {
	store SessionStore
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	entries map[int64]cachedSession
	// gen is incremented by Invalidate. A lookup only caches its result if
	// no invalidation happened while it was reading the store, so a value
	// read just before a revocation cannot be written back afterwards.
	gen uint64
}

type cachedSession struct {
	version   int64
	fetchedAt time.Time
}

func NewCachedSessions(store SessionStore, ttl time.Duration) *CachedSessions {
	return &CachedSessions{store: store, ttl: ttl, now: time.Now, entries: map[int64]cachedSession{}}
}

func (c *CachedSessions) SessionVersion(ctx context.Context, userID int64) (int64, error) {
	c.mu.Lock()
	e, ok := c.entries[userID]
	gen := c.gen
	c.mu.Unlock()
	if ok && c.now().Sub(e.fetchedAt) < c.ttl {
		return e.version, nil
	}

	v, err := c.store.SessionVersion(ctx, userID)
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	if c.gen == gen { // no Invalidate raced with the read
		if len(c.entries) >= maxCachedSessions {
			clear(c.entries)
		}
		c.entries[userID] = cachedSession{version: v, fetchedAt: c.now()}
	}
	c.mu.Unlock()
	return v, nil
}

// Invalidate drops the cached version for a user, so a revocation made by
// this instance applies to its next request immediately. (Other instances
// still pick it up within ttl.)
func (c *CachedSessions) Invalidate(userID int64) {
	c.mu.Lock()
	delete(c.entries, userID)
	c.gen++
	c.mu.Unlock()
}
