// Package database opens the MySQL connection pool and applies migrations.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"go-crud/config"
	"go-crud/migrations"
)

func mysqlConfig(cfg config.Config) *mysql.Config {
	c := mysql.NewConfig()
	c.User = cfg.DBUser
	c.Passwd = cfg.DBPassword
	c.Net = "tcp"
	c.Addr = cfg.DBAddr
	c.DBName = cfg.DBName
	// Report matched rather than changed rows, so an UPDATE that leaves a
	// row unchanged is not mistaken for a missing row.
	c.ClientFoundRows = true
	// Scan DATETIME columns into time.Time (UTC, the driver's default Loc).
	c.ParseTime = true
	c.Timeout = 5 * time.Second
	c.ReadTimeout = 30 * time.Second
	c.WriteTimeout = 30 * time.Second
	return c
}

// Open connects to MySQL, retrying until ctx is done so the app can start
// while the database container is still booting.
func Open(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	connector, err := mysql.NewConnector(mysqlConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("mysql config: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)

	if err := pingWithRetry(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func pingWithRetry(ctx context.Context, db *sql.DB) error {
	delay := 500 * time.Millisecond
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		slog.Warn("database not ready, retrying", "error", err, "retry_in", delay)
		select {
		case <-ctx.Done():
			return fmt.Errorf("connect to database: %w (last error: %w)", ctx.Err(), err)
		case <-time.After(delay):
		}
		delay = min(delay*2, 5*time.Second)
	}
}

// Migrate applies all pending migrations embedded in the binary, first
// waiting (until ctx is done) for the database to accept connections. It
// uses a dedicated connection with multi-statement support, which the main
// pool deliberately does not enable.
func Migrate(ctx context.Context, cfg config.Config) error {
	mc := mysqlConfig(cfg)
	mc.MultiStatements = true
	connector, err := mysql.NewConnector(mc)
	if err != nil {
		return fmt.Errorf("mysql config: %w", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	if err := pingWithRetry(ctx, db); err != nil {
		db.Close()
		return err
	}

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		db.Close()
		return fmt.Errorf("migration driver: %w", err)
	}
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		driver.Close()
		return fmt.Errorf("migration source: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		src.Close()
		driver.Close()
		return fmt.Errorf("migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	version, dirty, _ := m.Version()
	slog.Info("database migrations applied", "version", version, "dirty", dirty)
	return nil
}
