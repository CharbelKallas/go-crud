package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestIPRateLimiterBurstAndRefill(t *testing.T) {
	l := NewIPRateLimiter(3)
	now := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if ok, _ := l.allow("1.2.3.4"); !ok {
			t.Fatalf("request %d refused within burst", i+1)
		}
	}
	ok, wait := l.allow("1.2.3.4")
	if ok || wait <= 0 || wait > 20*time.Second {
		t.Fatalf("4th request: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.allow("5.6.7.8"); !ok {
		t.Fatal("other client limited")
	}

	now = now.Add(20 * time.Second) // one token refilled
	if ok, _ := l.allow("1.2.3.4"); !ok {
		t.Fatal("not refilled after 20s")
	}
	if ok, _ := l.allow("1.2.3.4"); ok {
		t.Fatal("refilled more than one token")
	}
}

func TestIPRateLimiterForgetsIdleClients(t *testing.T) {
	l := NewIPRateLimiter(1)
	now := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return now }
	l.allow("1.2.3.4")
	now = now.Add(2 * time.Minute)
	l.allow("5.6.7.8") // triggers a sweep
	if _, ok := l.clients["1.2.3.4"]; ok {
		t.Fatal("idle client not swept")
	}
}

func TestClientKeyGroupsIPv6By64(t *testing.T) {
	a := clientKey("2001:db8:1:2:aaaa::1")
	b := clientKey("2001:db8:1:2:bbbb::2")
	c := clientKey("2001:db8:1:3::1")
	if a != b || a == c {
		t.Fatalf("keys: %s %s %s", a, b, c)
	}
	if clientKey("10.0.0.1") != "10.0.0.1" {
		t.Fatal("IPv4 key changed")
	}
}

func TestMiddlewareAnswers429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/login", NewIPRateLimiter(1).Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

	codes := []int{}
	var retry string
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/login", nil))
		codes = append(codes, rec.Code)
		retry = rec.Header().Get("Retry-After")
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusTooManyRequests || retry == "" {
		t.Fatalf("codes=%v retry-after=%q", codes, retry)
	}
}
