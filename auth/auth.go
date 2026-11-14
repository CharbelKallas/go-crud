// Package auth issues and validates the JWTs used by the API.
//
// Two kinds of token are issued: short-lived access tokens, accepted by the
// API routes, and longer-lived refresh tokens, accepted only by the refresh
// endpoint. The kind is stored in the "typ" claim so one can never be used
// in place of the other.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
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

// Subject identifies whom a token is issued to.
type Subject struct {
	UserID int64
	Email  string
	// SessionVersion is the user's session version at issue time. Revoking
	// a user's sessions increments it, which invalidates every token
	// carrying an older version without comparing clocks.
	SessionVersion int64
}

type Claims struct {
	UserID         int64     `json:"uid"`
	SessionVersion int64     `json:"sv"`
	Email          string    `json:"email"`
	Type           TokenType `json:"typ"`
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

// Issued is a signed token with the identifiers needed to track it.
type Issued struct {
	Token     string
	ID        string // the "jti" claim
	ExpiresAt time.Time
}

// Generate signs a new token of the given type for the user.
func (m *TokenManager) Generate(sub Subject, typ TokenType) (string, error) {
	issued, err := m.Issue(sub, typ)
	return issued.Token, err
}

// Issue signs a new token of the given type and returns it with its unique
// ID and expiry, so refresh tokens can be recorded server-side.
func (m *TokenManager) Issue(sub Subject, typ TokenType) (Issued, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Issued{}, fmt.Errorf("token id: %w", err)
	}
	ttl, err := m.ttl(typ)
	if err != nil {
		return Issued{}, err
	}
	return m.IssueWithID(sub, typ, hex.EncodeToString(idBytes), m.now().Add(ttl))
}

// IssueWithID signs a token with a given ID and expiry. It re-issues an
// already recorded refresh token (same ID, same expiry) when a client
// retries a refresh.
func (m *TokenManager) IssueWithID(sub Subject, typ TokenType, id string, expiresAt time.Time) (Issued, error) {
	if sub.UserID <= 0 || sub.Email == "" {
		return Issued{}, errors.New("token subject needs a user ID and an email")
	}
	if _, err := m.ttl(typ); err != nil {
		return Issued{}, err
	}
	now := m.now()
	claims := Claims{
		UserID:         sub.UserID,
		SessionVersion: sub.SessionVersion,
		Email:          sub.Email,
		Type:           typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        id,
			Subject:   strconv.FormatInt(sub.UserID, 10),
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return Issued{}, err
	}
	return Issued{Token: signed, ID: id, ExpiresAt: expiresAt}, nil
}

func (m *TokenManager) ttl(typ TokenType) (time.Duration, error) {
	var ttl time.Duration
	switch typ {
	case AccessToken:
		ttl = m.accessTTL
	case RefreshToken:
		ttl = m.refreshTTL
	default:
		return 0, fmt.Errorf("unknown token type %q", typ)
	}
	return ttl, nil
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
	if claims.ID == "" {
		return nil, fmt.Errorf("%w: missing jti", ErrInvalidToken)
	}
	if claims.UserID <= 0 || claims.Email == "" {
		return nil, fmt.Errorf("%w: missing user", ErrInvalidToken)
	}
	return claims, nil
}
