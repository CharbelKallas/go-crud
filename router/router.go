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
	DB      handlers.Pinger
	// AuthRateLimitPerMinute limits the /api/auth routes per client IP;
	// 0 disables the limit (tests).
	AuthRateLimitPerMinute int
}

func New(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(middlewares.RequestLogger(), gin.Recovery(), middlewares.MaxBodySize(maxBodyBytes))
	// The app is not deployed behind a trusted proxy; do not believe
	// X-Forwarded-For.
	_ = r.SetTrustedProxies(nil)

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Welcome To This Website")
	})
	r.GET("/healthz", handlers.Health(d.DB))

	api := r.Group("/api")

	public := api.Group("/auth")
	if d.AuthRateLimitPerMinute > 0 {
		public.Use(middlewares.NewIPRateLimiter(d.AuthRateLimitPerMinute).Middleware())
	}
	public.POST("/login", d.Auth.Login)
	public.POST("/signup", d.Auth.Signup)
	public.POST("/refresh", d.Auth.Refresh)

	requireAuth := middlewares.AuthHandler(d.Tokens)

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
