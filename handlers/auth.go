package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"go-crud/auth"
	"go-crud/models"
	"go-crud/repositories"
)

// bcrypt only uses the first 72 bytes of a password; longer ones are
// rejected rather than silently truncated.
const maxPasswordBytes = 72

type UserStore interface {
	Create(ctx context.Context, user models.User) error
	GetByEmail(ctx context.Context, email string) (models.User, error)
	UpdatePasswordHash(ctx context.Context, id int64, hash string) error
}

type AuthHandler struct {
	users      UserStore
	tokens     *auth.TokenManager
	bcryptCost int
	// dummyHash is compared against when the user does not exist, so a
	// login for an unknown email takes as long as one with a wrong
	// password and does not reveal which emails are registered.
	dummyHash string
}

func NewAuthHandler(users UserStore, tokens *auth.TokenManager, bcryptCost int) (*AuthHandler, error) {
	dummy, err := bcrypt.GenerateFromPassword([]byte("timing-equalisation-dummy"), bcryptCost)
	if err != nil {
		return nil, err
	}
	return &AuthHandler{users: users, tokens: tokens, bcryptCost: bcryptCost, dummyHash: string(dummy)}, nil
}

type SignupPayload struct {
	Name     string `binding:"required,max=128"`
	Email    string `binding:"required,email,max=255"`
	Password string `binding:"required,min=8"`
}

type LoginPayload struct {
	Email    string `binding:"required"`
	Password string `binding:"required"`
}

type RefreshPayload struct {
	RefreshToken string `binding:"required"`
}

type LoginResponse struct {
	Token        string
	RefreshToken string
}

func (h *AuthHandler) Signup(c *gin.Context) {
	var p SignupPayload
	if !bindJSON(c, &p) {
		return
	}
	if len(p.Password) > maxPasswordBytes {
		respondError(c, http.StatusBadRequest, "Password must be at most 72 bytes")
		return
	}

	user := models.User{Name: strings.TrimSpace(p.Name), Email: strings.TrimSpace(p.Email)}
	if err := user.HashPassword(p.Password, h.bcryptCost); err != nil {
		respondInternal(c, err)
		return
	}

	err := h.users.Create(c.Request.Context(), user)
	if errors.Is(err, repositories.ErrDuplicateEmail) {
		respondError(c, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		respondInternal(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"Message": "Successfully Register"})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var p LoginPayload
	if !bindJSON(c, &p) {
		return
	}

	user, err := h.users.GetByEmail(c.Request.Context(), strings.TrimSpace(p.Email))
	switch {
	case errors.Is(err, repositories.ErrNotFound):
		_ = bcrypt.CompareHashAndPassword([]byte(h.dummyHash), []byte(p.Password))
		respondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	case err != nil:
		respondInternal(c, err)
		return
	}
	if err := user.CheckPassword(p.Password); err != nil {
		respondError(c, http.StatusUnauthorized, "invalid credentials")
		return
	}
	h.upgradeHash(c.Request.Context(), user, p.Password)
	h.issueTokens(c, user.Email)
}

// Refresh exchanges a valid refresh token for a new access/refresh pair.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var p RefreshPayload
	if !bindJSON(c, &p) {
		return
	}
	claims, err := h.tokens.Validate(p.RefreshToken, auth.RefreshToken)
	if err != nil {
		slog.InfoContext(c.Request.Context(), "refresh token rejected", "error", err)
		respondError(c, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	// Make sure the account still exists before issuing new tokens.
	user, err := h.users.GetByEmail(c.Request.Context(), claims.Email)
	if errors.Is(err, repositories.ErrNotFound) {
		respondError(c, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if err != nil {
		respondInternal(c, err)
		return
	}
	h.issueTokens(c, user.Email)
}

// upgradeHash re-hashes the password when it was stored with a different
// bcrypt cost (accounts created before the cost became configurable used
// 14). This keeps login time uniform. Failures are logged, not fatal.
func (h *AuthHandler) upgradeHash(ctx context.Context, user models.User, password string) {
	cost, err := bcrypt.Cost([]byte(user.Password))
	if err != nil || cost == h.bcryptCost {
		return
	}
	if err := user.HashPassword(password, h.bcryptCost); err != nil {
		slog.WarnContext(ctx, "password re-hash failed", "error", err)
		return
	}
	if err := h.users.UpdatePasswordHash(ctx, user.ID, user.Password); err != nil {
		slog.WarnContext(ctx, "password re-hash failed", "error", err)
	}
}

func (h *AuthHandler) issueTokens(c *gin.Context, email string) {
	access, err := h.tokens.Generate(email, auth.AccessToken)
	if err != nil {
		respondInternal(c, err)
		return
	}
	refresh, err := h.tokens.Generate(email, auth.RefreshToken)
	if err != nil {
		respondInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, LoginResponse{Token: access, RefreshToken: refresh})
}
