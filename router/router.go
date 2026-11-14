// Package router wires the HTTP routes.
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"go-crud/auth"
	"go-crud/handlers"
	"go-crud/middlewares"
)

const maxBodyBytes = 1 << 20

type Deps struct {
	Tokens  *auth.TokenManager
	Auth    *handlers.AuthHandler
	Albums  *handlers.AlbumHandler
	Weather *handlers.WeatherHandler
	Health  *handlers.HealthCheck
	// Sessions is checked on every authenticated request (deleted users,
	// revoked sessions); wrap it in middlewares.CachedSessions to limit
	// database lookups.
	Sessions middlewares.SessionStore
	// AuthRateLimitPerMinute limits the /api/auth routes per client IP;
	// 0 disables the limit (tests).
	AuthRateLimitPerMinute int
	// TrustedProxies are the reverse proxies whose forwarding headers are
	// believed for the client IP (used by the rate limiter and logs).
	TrustedProxies []string
}

func New(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(middlewares.RequestLogger(), gin.Recovery(), middlewares.MaxBodySize(maxBodyBytes))
	// Only believe X-Forwarded-For / X-Real-IP from configured proxies;
	// with none, the TCP peer address is the client IP. Entries are
	// validated by config.Load.
	if err := r.SetTrustedProxies(d.TrustedProxies); err != nil {
		panic(err)
	}

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Welcome To This Website")
	})
	r.GET("/healthz", d.Health.Handle)

	api := r.Group("/api")

	public := api.Group("/auth")
	if d.AuthRateLimitPerMinute > 0 {
		public.Use(middlewares.NewIPRateLimiter(d.AuthRateLimitPerMinute).Middleware())
	}
	public.POST("/login", d.Auth.Login)
	public.POST("/signup", d.Auth.Signup)
	public.POST("/refresh", d.Auth.Refresh)

	requireAuth := middlewares.AuthHandler(d.Tokens, d.Sessions)

	albums := api.Group("/albums", requireAuth)
	// Both "/api/albums" and "/api/albums/" are served directly: a
	// trailing-slash redirect would make some clients drop the
	// Authorization header or the request body.
	for _, root := range []string{"", "/"} {
		albums.GET(root, d.Albums.List)
		albums.POST(root, d.Albums.Create)
	}
	albums.GET("/:id", d.Albums.Get)
	albums.PUT("/:id", d.Albums.Update)
	albums.DELETE("/:id", d.Albums.Delete)
	albums.GET("/artist/:artist", d.Albums.ListByArtist)

	api.GET("/weather/:lat/:long", requireAuth, d.Weather.Current)

	return r
}
