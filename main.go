package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-crud/auth"
	"go-crud/config"
	"go-crud/database"
	"go-crud/handlers"
	"go-crud/repositories"
	"go-crud/router"
	"go-crud/weather"
)

// Usage:
//
//	go-crud           serve the API
//	go-crud migrate   apply database migrations and exit
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	run := serve
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			run = migrate
		default:
			slog.Error("unknown command", "command", os.Args[1])
			os.Exit(2)
		}
	}
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// migrate applies pending migrations and exits. docker-compose runs it as a
// one-shot service before the API starts, so a failed migration stops the
// stack with a clear error instead of crash-looping the API.
// It needs only the database settings.
func migrate() error {
	cfg, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return database.Migrate(ctx, cfg)
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	db, err := database.Open(connectCtx, cfg)
	cancel()
	if err != nil {
		return err
	}
	defer db.Close()
	slog.Info("connected to database", "addr", cfg.DBAddr, "database", cfg.DBName)

	if cfg.MigrateOnStart {
		if err := database.Migrate(ctx, cfg); err != nil {
			return err
		}
	}

	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	authHandler, err := handlers.NewAuthHandler(repositories.NewUserRepository(db), repositories.NewRefreshTokenRepository(db, cfg.RefreshReuseGrace), tokens, cfg.BcryptCost)
	if err != nil {
		return err
	}

	var weatherProvider handlers.WeatherProvider
	if cfg.WeatherAPIKey != "" {
		var transport http.RoundTripper
		if cfg.WeatherSigningSecret != "" {
			transport = &weather.SigningTransport{Secret: cfg.WeatherSigningSecret, ClientID: cfg.WeatherSigningClientID}
		}
		weatherProvider = weather.NewClient(cfg.WeatherAPIBaseURL, cfg.WeatherAPIKey, transport)
	} else {
		slog.Warn("WEATHER_API_KEY not set: /api/weather will answer 503")
	}

	engine := router.New(router.Deps{
		Tokens:  tokens,
		Auth:    authHandler,
		Albums:  handlers.NewAlbumHandler(repositories.NewAlbumRepository(db)),
		Weather: handlers.NewWeatherHandler(weatherProvider),
		DB:      db,

		AuthRateLimitPerMinute: cfg.AuthRateLimitPerMinute,
		TrustedProxies:         cfg.TrustedProxies,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	return nil
}
