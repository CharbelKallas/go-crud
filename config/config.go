// Package config loads the application configuration from environment
// variables. Secrets are never given defaults: the app refuses to start
// when a required secret is missing or too weak.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// MinJWTSecretLength is the minimum length, in bytes, of JWT_SECRET.
// HS256 keys should be at least as long as the hash output (32 bytes).
const MinJWTSecretLength = 32

type Config struct {
	HTTPAddr string

	DBUser     string
	DBPassword string
	DBAddr     string
	DBName     string
	// MigrateOnStart applies the embedded SQL migrations at startup.
	MigrateOnStart bool

	JWTSecret       string
	JWTIssuer       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	BcryptCost      int
	// AuthRateLimitPerMinute is how many login/signup/refresh requests
	// one client IP may make per minute.
	AuthRateLimitPerMinute int
	// RefreshReuseGrace lets a just-used refresh token be presented again
	// for this long (client retries); 0 makes refresh tokens strictly
	// single-use.
	RefreshReuseGrace time.Duration
	// TrustedProxies lists the reverse proxies (IPs or CIDRs) whose
	// X-Forwarded-For / X-Real-IP headers are believed when determining
	// the client IP. Empty means none: the TCP peer is the client.
	TrustedProxies []string

	// WeatherAPIKey is the weatherapi.com key. When empty the weather
	// endpoint answers 503 instead of failing the whole app.
	WeatherAPIKey     string
	WeatherAPIBaseURL string
	// Optional request-signing credentials for the weather API. The
	// signing transport is only installed when both are set.
	WeatherSigningSecret   string
	WeatherSigningClientID string
}

// Load reads the configuration from the environment and validates it.
func Load() (Config, error) {
	var errs []error

	cfg := Config{
		HTTPAddr:               getenv("HTTP_ADDR", ":8080"),
		DBUser:                 os.Getenv("DB_USERNAME"),
		DBPassword:             os.Getenv("DB_PASSWORD"),
		DBAddr:                 getenv("DATABASE_ADDR", "127.0.0.1:3306"),
		DBName:                 getenv("DB_DATABASE", "recordings"),
		JWTSecret:              os.Getenv("JWT_SECRET"),
		JWTIssuer:              getenv("JWT_ISSUER", "go-crud"),
		WeatherAPIKey:          os.Getenv("WEATHER_API_KEY"),
		WeatherAPIBaseURL:      getenv("WEATHER_API_BASE_URL", "https://api.weatherapi.com/v1"),
		WeatherSigningSecret:   os.Getenv("WEATHER_SIGNING_SECRET"),
		WeatherSigningClientID: os.Getenv("WEATHER_SIGNING_CLIENT_ID"),
	}

	var err error
	if cfg.MigrateOnStart, err = strconv.ParseBool(getenv("MIGRATE_ON_START", "true")); err != nil {
		errs = append(errs, fmt.Errorf("MIGRATE_ON_START: %w", err))
	}
	if cfg.AccessTokenTTL, err = time.ParseDuration(getenv("ACCESS_TOKEN_TTL", "15m")); err != nil || cfg.AccessTokenTTL <= 0 {
		errs = append(errs, fmt.Errorf("ACCESS_TOKEN_TTL must be a positive duration such as 15m"))
	}
	if cfg.RefreshTokenTTL, err = time.ParseDuration(getenv("REFRESH_TOKEN_TTL", "12h")); err != nil || cfg.RefreshTokenTTL <= 0 {
		errs = append(errs, fmt.Errorf("REFRESH_TOKEN_TTL must be a positive duration such as 12h"))
	}
	if cfg.BcryptCost, err = strconv.Atoi(getenv("BCRYPT_COST", "12")); err != nil ||
		cfg.BcryptCost < bcrypt.DefaultCost || cfg.BcryptCost > bcrypt.MaxCost {
		errs = append(errs, fmt.Errorf("BCRYPT_COST must be an integer between %d and %d", bcrypt.DefaultCost, bcrypt.MaxCost))
	}

	if cfg.AuthRateLimitPerMinute, err = strconv.Atoi(getenv("AUTH_RATE_LIMIT_PER_MINUTE", "10")); err != nil ||
		cfg.AuthRateLimitPerMinute < 1 || cfg.AuthRateLimitPerMinute > 10_000 {
		errs = append(errs, errors.New("AUTH_RATE_LIMIT_PER_MINUTE must be an integer between 1 and 10000"))
	}

	if cfg.RefreshReuseGrace, err = time.ParseDuration(getenv("REFRESH_REUSE_GRACE", "10s")); err != nil ||
		cfg.RefreshReuseGrace < 0 || cfg.RefreshReuseGrace > time.Minute {
		errs = append(errs, errors.New("REFRESH_REUSE_GRACE must be a duration between 0s and 1m"))
	}
	for _, p := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if net.ParseIP(p) == nil {
			if _, _, err := net.ParseCIDR(p); err != nil {
				errs = append(errs, fmt.Errorf("TRUSTED_PROXIES: %q is not an IP address or CIDR", p))
				continue
			}
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, p)
	}

	if cfg.DBUser == "" {
		errs = append(errs, errors.New("DB_USERNAME is required"))
	}
	if cfg.DBPassword == "" {
		errs = append(errs, errors.New("DB_PASSWORD is required"))
	}
	if len(cfg.JWTSecret) < MinJWTSecretLength {
		errs = append(errs, fmt.Errorf("JWT_SECRET is required and must be at least %d bytes (generate one with: openssl rand -base64 48)", MinJWTSecretLength))
	}
	if (cfg.WeatherSigningSecret == "") != (cfg.WeatherSigningClientID == "") {
		errs = append(errs, errors.New("WEATHER_SIGNING_SECRET and WEATHER_SIGNING_CLIENT_ID must be set together"))
	}

	return cfg, errors.Join(errs...)
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
