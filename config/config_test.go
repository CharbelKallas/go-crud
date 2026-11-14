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
		"ACCESS_TOKEN_TTL", "REFRESH_TOKEN_TTL", "BCRYPT_COST", "WEATHER_API_KEY",
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
		cfg.BcryptCost != 12 || !cfg.MigrateOnStart || !strings.HasPrefix(cfg.WeatherAPIBaseURL, "https://") {
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
