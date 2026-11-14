// Package auth issues and validates the JWTs used by the API.
//
// Two kinds of token are issued: short-lived access tokens, accepted by the
// API routes, and longer-lived refresh tokens, accepted only by the refresh
// endpoint. The kind is stored in the "typ" claim so one can never be used
// in place of the other.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// ErrInvalidToken is returned for any token that fails validation. The
// underlying reason is wrapped for logging but should not be shown to
// clients.
var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	Email string    `json:"email"`
	Type  TokenType `json:"typ"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

func NewTokenManager(secret, issuer string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{
		secret:     []byte(secret),
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}
}

// Generate signs a new token of the given type for the user.
func (m *TokenManager) Generate(email string, typ TokenType) (string, error) {
	var ttl time.Duration
	switch typ {
	case AccessToken:
		ttl = m.accessTTL
	case RefreshToken:
		ttl = m.refreshTTL
	default:
		return "", fmt.Errorf("unknown token type %q", typ)
	}

	now := m.now()
	claims := Claims{
		Email: email,
		Type:  typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   email,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Validate parses the token and checks its signature, algorithm, issuer,
// expiry and type.
func (m *TokenManager) Validate(signed string, want TokenType) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(signed, claims,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if claims.Type != want {
		return nil, fmt.Errorf("%w: expected %s token, got %q", ErrInvalidToken, want, claims.Type)
	}
	if claims.Email == "" {
		return nil, fmt.Errorf("%w: missing email", ErrInvalidToken)
	}
	return claims, nil
}
