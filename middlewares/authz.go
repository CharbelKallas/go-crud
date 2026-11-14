// Package middlewares holds Gin middlewares.
package middlewares

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"go-crud/auth"
)

// AuthHandler requires a valid access token in "Authorization: Bearer <token>".
// Refresh tokens are rejected.
func AuthHandler(tokens *auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, found := strings.Cut(c.GetHeader("Authorization"), " ")
		token = strings.TrimSpace(token)
		if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
			unauthorized(c, "missing or malformed Authorization header")
			return
		}

		_, err := tokens.Validate(token, auth.AccessToken)
		if err != nil {
			slog.InfoContext(c.Request.Context(), "access token rejected", "error", err)
			unauthorized(c, "invalid or expired token")
			return
		}

		c.Next()
	}
}

func unauthorized(c *gin.Context, msg string) {
	c.Header("WWW-Authenticate", `Bearer realm="go-crud"`)
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": msg})
}
