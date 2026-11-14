// Package middlewares holds Gin middlewares.
package middlewares

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"go-crud/auth"
	"go-crud/repositories"
)

// SessionStore reports when a user's sessions were revoked (see
// repositories.UserRepository.TokensRevokedAt).
type SessionStore interface {
	TokensRevokedAt(ctx context.Context, email string) (time.Time, error)
}

// AuthHandler requires a valid access token in "Authorization: Bearer <token>".
// Refresh tokens are rejected, as are access tokens of deleted users and
// tokens issued before the user's sessions were revoked.
func AuthHandler(tokens *auth.TokenManager, sessions SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, token, found := strings.Cut(c.GetHeader("Authorization"), " ")
		token = strings.TrimSpace(token)
		if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
			unauthorized(c, "missing or malformed Authorization header")
			return
		}

		claims, err := tokens.Validate(token, auth.AccessToken)
		if err != nil {
			slog.InfoContext(c.Request.Context(), "access token rejected", "error", err)
			unauthorized(c, "invalid or expired token")
			return
		}

		revokedAt, err := sessions.TokensRevokedAt(c.Request.Context(), claims.Email)
		switch {
		case errors.Is(err, repositories.ErrNotFound):
			unauthorized(c, "invalid or expired token")
			return
		case err != nil:
			slog.ErrorContext(c.Request.Context(), "session check failed", "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
			return
		case !revokedAt.IsZero() && (claims.IssuedAt == nil || !claims.IssuedAt.Time.After(revokedAt)):
			slog.InfoContext(c.Request.Context(), "access token rejected: sessions revoked")
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
