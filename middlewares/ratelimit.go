package middlewares

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

const (
	// maxTrackedClients bounds the limiter's memory. When it is full, new
	// clients are refused until idle entries expire: failing closed keeps
	// a flood of spoofed source addresses from disabling the limit.
	maxTrackedClients = 100_000
	sweepInterval     = time.Minute
)

// IPRateLimiter is a per-client token bucket kept in memory. Each client
// may make up to perMinute requests in a burst, refilled evenly over a
// minute. It limits a single instance only; run several replicas behind a
// load balancer and each one keeps its own counts.
type IPRateLimiter struct {
	mu        sync.Mutex
	limit     rate.Limit
	burst     int
	idleTTL   time.Duration
	clients   map[string]*clientBucket
	lastSweep time.Time
	now       func() time.Time
}

type clientBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewIPRateLimiter(perMinute int) *IPRateLimiter {
	return &IPRateLimiter{
		limit: rate.Every(time.Minute / time.Duration(perMinute)),
		burst: perMinute,
		// After a minute idle a bucket is full again, so forgetting it
		// changes nothing.
		idleTTL: time.Minute,
		clients: map[string]*clientBucket{},
		now:     time.Now,
	}
}

// Middleware answers 429 with Retry-After when the client is over its limit.
func (l *IPRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ok, wait := l.allow(clientKey(c.ClientIP()))
		if !ok {
			c.Header("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"message": "too many requests, try again later"})
			return
		}
		c.Next()
	}
}

func (l *IPRateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.Sub(l.lastSweep) >= sweepInterval {
		for k, b := range l.clients {
			if now.Sub(b.lastSeen) > l.idleTTL {
				delete(l.clients, k)
			}
		}
		l.lastSweep = now
	}

	b, found := l.clients[key]
	if !found {
		if len(l.clients) >= maxTrackedClients {
			return false, sweepInterval
		}
		b = &clientBucket{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.clients[key] = b
	}
	b.lastSeen = now

	r := b.limiter.ReserveN(now, 1)
	if delay := r.DelayFrom(now); delay > 0 {
		r.CancelAt(now)
		return false, delay
	}
	return true, 0
}

// clientKey groups IPv6 clients by /64, the block a single subscriber
// usually controls, so rotating addresses inside it does not reset the
// limit.
func clientKey(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() != nil {
		return ip
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
