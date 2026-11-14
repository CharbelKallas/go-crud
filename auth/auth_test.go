package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-that-is-at-least-32-bytes-long"

func newManager() *TokenManager {
	return NewTokenManager(testSecret, "go-crud-test", 15*time.Minute, 12*time.Hour)
}

func TestGenerateAndValidate(t *testing.T) {
	m := newManager()
	for _, typ := range []TokenType{AccessToken, RefreshToken} {
		tok, err := m.Generate("a@example.com", typ)
		if err != nil {
			t.Fatalf("generate %s: %v", typ, err)
		}
		claims, err := m.Validate(tok, typ)
		if err != nil {
			t.Fatalf("validate %s: %v", typ, err)
		}
		if claims.Email != "a@example.com" || claims.Subject != "a@example.com" || claims.Type != typ {
			t.Errorf("unexpected claims %+v", claims)
		}
	}
}

func TestTokenTypeIsEnforced(t *testing.T) {
	m := newManager()
	refresh, _ := m.Generate("a@example.com", RefreshToken)
	if _, err := m.Validate(refresh, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh token accepted as access token: %v", err)
	}
	access, _ := m.Generate("a@example.com", AccessToken)
	if _, err := m.Validate(access, RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("access token accepted as refresh token: %v", err)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	m := newManager()
	m.now = func() time.Time { return time.Now().Add(-time.Hour) }
	tok, _ := m.Generate("a@example.com", AccessToken)
	m.now = time.Now
	if _, err := m.Validate(tok, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
}

func TestWrongSecretRejected(t *testing.T) {
	other := NewTokenManager("another-secret-that-is-at-least-32-bytes", "go-crud-test", time.Minute, time.Hour)
	tok, _ := other.Generate("a@example.com", AccessToken)
	if _, err := newManager().Validate(tok, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token signed with another secret accepted: %v", err)
	}
}

func TestWrongIssuerRejected(t *testing.T) {
	other := NewTokenManager(testSecret, "someone-else", time.Minute, time.Hour)
	tok, _ := other.Generate("a@example.com", AccessToken)
	if _, err := newManager().Validate(tok, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token from another issuer accepted: %v", err)
	}
}

func TestOtherAlgorithmsRejected(t *testing.T) {
	claims := Claims{Email: "a@example.com", Type: AccessToken, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "go-crud-test", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	hs512, _ := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte(testSecret))
	for name, tok := range map[string]string{"none": none, "HS512": hs512} {
		if _, err := newManager().Validate(tok, AccessToken); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s token accepted: %v", name, err)
		}
	}
}

func TestTokenWithoutExpiryRejected(t *testing.T) {
	claims := Claims{Email: "a@example.com", Type: AccessToken, RegisteredClaims: jwt.RegisteredClaims{Issuer: "go-crud-test"}}
	tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if _, err := newManager().Validate(tok, AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token without exp accepted: %v", err)
	}
}
