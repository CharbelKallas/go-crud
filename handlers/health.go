package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Pinger interface {
	PingContext(ctx context.Context) error
}

// healthCacheTTL is how long a database check result is reused, so the
// public health endpoint cannot be used to generate database load.
const healthCacheTTL = 5 * time.Second

// Health answers 200 when the database is reachable, 503 otherwise. The
// database is pinged at most once per healthCacheTTL; concurrent requests
// share that result.
func Health(db Pinger) gin.HandlerFunc {
	var (
		mu      sync.Mutex
		checked time.Time
		healthy bool
	)
	return func(c *gin.Context) {
		mu.Lock()
		if time.Since(checked) >= healthCacheTTL {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			err := db.PingContext(ctx)
			cancel()
			if err != nil {
				logErr(c, err)
			}
			healthy, checked = err == nil, time.Now()
		}
		ok := healthy
		mu.Unlock()

		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
