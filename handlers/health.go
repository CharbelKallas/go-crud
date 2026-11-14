package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Pinger interface {
	PingContext(ctx context.Context) error
}

const (
	// healthCacheTTL is how long a database check result is reused, so the
	// public health endpoint cannot be used to generate database load.
	healthCacheTTL    = 5 * time.Second
	healthPingTimeout = 2 * time.Second
)

// HealthCheck serves /healthz: 200 when the database is reachable, 503
// otherwise.
//
// The database is pinged at most once per healthCacheTTL. Requests arriving
// while a ping is running wait for that same ping instead of queueing their
// own, and the ping runs on the checker's own context, so a client that
// disconnects cannot make the check fail. Close stops it before the
// database is closed.
type HealthCheck struct {
	db     Pinger
	base   context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	checked time.Time
	healthy bool
	pending chan struct{} // closed when the running ping finishes; nil if none
}

func NewHealthCheck(db Pinger) *HealthCheck {
	base, cancel := context.WithCancel(context.Background())
	return &HealthCheck{db: db, base: base, cancel: cancel}
}

// Close cancels any running ping and waits for it to finish. Call it
// before closing the database.
func (h *HealthCheck) Close() {
	h.cancel()
	h.wg.Wait()
}

func (h *HealthCheck) Handle(c *gin.Context) {
	healthy, ok := h.status(c.Request.Context())
	if !ok || !healthy {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// status returns the cached result, refreshing it when stale. ok is false
// when ctx ended (or the checker was closed) before a result was available.
func (h *HealthCheck) status(ctx context.Context) (healthy, ok bool) {
	h.mu.Lock()
	if time.Since(h.checked) < healthCacheTTL {
		healthy = h.healthy
		h.mu.Unlock()
		return healthy, true
	}
	done := h.pending
	if done == nil {
		if h.base.Err() != nil { // closed
			h.mu.Unlock()
			return false, false
		}
		done = make(chan struct{})
		h.pending = done
		h.wg.Add(1)
		go h.ping(done)
	}
	h.mu.Unlock()

	select {
	case <-done:
	case <-ctx.Done():
		return false, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.healthy, true
}

func (h *HealthCheck) ping(done chan struct{}) {
	defer h.wg.Done()
	ctx, cancel := context.WithTimeout(h.base, healthPingTimeout)
	err := h.db.PingContext(ctx)
	cancel()
	if err != nil && h.base.Err() == nil { // not shutting down
		slog.Error("health check: database unreachable", "error", err)
	}

	h.mu.Lock()
	h.healthy, h.checked, h.pending = err == nil, time.Now(), nil
	h.mu.Unlock()
	close(done)
}
