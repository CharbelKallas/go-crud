// Package middlewares holds Gin middlewares.
package middlewares

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"go-crud/auth"
	"go-crud/repositories"
)

// SessionStore returns a user's current session version (see
// repositories.UserRepository.SessionVersion), or repositories.ErrNotFound
// for a deleted user.
type SessionStore interface {
	SessionVersion(ctx context.Context, userID int64) (int64, error)
}

// AuthHandler requires a valid access token in "Authorization: Bearer <token>".
// Refresh tokens are rejected, as are access tokens of deleted users and
// tokens whose session version is older than the user's (sessions revoked).
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

		current, err := sessions.SessionVersion(c.Request.Context(), claims.UserID)
		switch {
		case errors.Is(err, repositories.ErrNotFound):
			unauthorized(c, "invalid or expired token")
			return
		case err != nil:
			slog.ErrorContext(c.Request.Context(), "session check failed", "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
			return
		case claims.SessionVersion < current:
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
