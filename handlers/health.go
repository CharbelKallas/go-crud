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

// Health answers 200 when the database is reachable, 503 otherwise.
//
// The database is pinged at most once per healthCacheTTL. Requests arriving
// while a ping is running wait for that same ping instead of queueing their
// own, and the ping runs on its own context, so a client that disconnects
// cannot make the check fail.
func Health(db Pinger) gin.HandlerFunc {
	hc := &healthCheck{db: db}
	return func(c *gin.Context) {
		healthy, ok := hc.status(c.Request.Context())
		if !ok || !healthy {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

type healthCheck struct {
	db Pinger

	mu      sync.Mutex
	checked time.Time
	healthy bool
	pending chan struct{} // closed when the running ping finishes; nil if none
}

// status returns the cached result, refreshing it when stale. ok is false
// when ctx ended before a result was available.
func (h *healthCheck) status(ctx context.Context) (healthy, ok bool) {
	h.mu.Lock()
	if time.Since(h.checked) < healthCacheTTL {
		healthy = h.healthy
		h.mu.Unlock()
		return healthy, true
	}
	done := h.pending
	if done == nil {
		done = make(chan struct{})
		h.pending = done
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

func (h *healthCheck) ping(done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), healthPingTimeout)
	err := h.db.PingContext(ctx)
	cancel()
	if err != nil {
		slog.Error("health check: database unreachable", "error", err)
	}

	h.mu.Lock()
	h.healthy, h.checked, h.pending = err == nil, time.Now(), nil
	h.mu.Unlock()
	close(done)
}
