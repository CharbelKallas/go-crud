package config

import (
	"strings"
	"testing"
	"time"
)

// setValidEnv sets the required variables and blanks every optional one
// (empty means "use the default"), so values exported in the developer's
// shell cannot change the results.
func setValidEnv(t *testing.T) {
	for _, k := range []string{
		"HTTP_ADDR", "DATABASE_ADDR", "DB_DATABASE", "MIGRATE_ON_START", "JWT_ISSUER",
		"ACCESS_TOKEN_TTL", "REFRESH_TOKEN_TTL", "BCRYPT_COST", "AUTH_RATE_LIMIT_PER_MINUTE", "REFRESH_REUSE_GRACE", "TRUSTED_PROXIES", "WEATHER_API_KEY",
		"WEATHER_API_BASE_URL", "WEATHER_SIGNING_SECRET", "WEATHER_SIGNING_CLIENT_ID",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("DB_USERNAME", "app")
	t.Setenv("DB_PASSWORD", "pw")
	t.Setenv("JWT_SECRET", strings.Repeat("x", MinJWTSecretLength))
}

func TestLoadDefaults(t *testing.T) {
	setValidEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.AccessTokenTTL != 15*time.Minute || cfg.RefreshTokenTTL != 12*time.Hour ||
		cfg.BcryptCost != 12 || cfg.AuthRateLimitPerMinute != 10 || cfg.RefreshReuseGrace != 10*time.Second || len(cfg.TrustedProxies) != 0 || !cfg.MigrateOnStart || !strings.HasPrefix(cfg.WeatherAPIBaseURL, "https://") {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadRejectsMissingOrWeakSecrets(t *testing.T) {
	tests := map[string]func(t *testing.T){
		"no jwt secret":    func(t *testing.T) { t.Setenv("JWT_SECRET", "") },
		"short jwt secret": func(t *testing.T) { t.Setenv("JWT_SECRET", "undersecretary") },
		"no db password":   func(t *testing.T) { t.Setenv("DB_PASSWORD", "") },
		"bad ttl":          func(t *testing.T) { t.Setenv("ACCESS_TOKEN_TTL", "-1m") },
		"low bcrypt cost":  func(t *testing.T) { t.Setenv("BCRYPT_COST", "4") },
		"zero rate limit":  func(t *testing.T) { t.Setenv("AUTH_RATE_LIMIT_PER_MINUTE", "0") },
		"long grace":       func(t *testing.T) { t.Setenv("REFRESH_REUSE_GRACE", "5m") },
		"bad proxy":        func(t *testing.T) { t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, nginx") },
		"half signing cfg": func(t *testing.T) { t.Setenv("WEATHER_SIGNING_SECRET", "s") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			setValidEnv(t)
			mutate(t)
			if _, err := Load(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLoadTrustedProxies(t *testing.T) {
	setValidEnv(t)
	t.Setenv("TRUSTED_PROXIES", " 10.0.0.0/8 , 192.0.2.7,,2001:db8::/32 ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.0/8", "192.0.2.7", "2001:db8::/32"}
	if strings.Join(cfg.TrustedProxies, " ") != strings.Join(want, " ") {
		t.Fatalf("TrustedProxies = %v", cfg.TrustedProxies)
	}
}
