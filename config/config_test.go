package config

import (
	"strings"
	"testing"
	"time"
)

func setValidEnv(t *testing.T) {
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
