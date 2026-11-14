package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// pruneInterval is how often Save also deletes expired rows.
const pruneInterval = time.Minute

// RefreshTokenRepository records issued refresh tokens so that each one can
// be used only once and a user's tokens can be revoked together.
type RefreshTokenRepository struct {
	db *sql.DB
	// reuseGrace is how long after its first use a refresh token may be
	// presented again (rounded to whole seconds). It lets a client retry
	// a refresh whose response it never received without triggering
	// theft detection. Zero makes tokens strictly single-use.
	reuseGrace time.Duration

	pruneMu   sync.Mutex
	lastPrune time.Time
}

func NewRefreshTokenRepository(db *sql.DB, reuseGrace time.Duration) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db, reuseGrace: reuseGrace}
}

// Save records a newly issued token. At most once a minute it also prunes
// expired rows so the table does not grow without bound.
func (r *RefreshTokenRepository) Save(ctx context.Context, id string, userID int64, expiresAt time.Time) error {
	r.maybePrune(ctx)
	if err := insertRefreshToken(ctx, r.db, id, userID, expiresAt); err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

// Rotate exchanges the refresh token oldID for newID in one transaction:
// either the old token is marked used and the new one stored, or nothing
// changes, so a failure never burns the client's token.
//
// It returns ErrNotFound for an unknown token. When the old token was
// already used (outside the grace window), revoked or expired it returns
// ErrTokenReused after revoking every refresh token of the user, because a
// second use means the token was copied.
func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldID string, userID int64, newID string, newExpiresAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}
	defer tx.Rollback()

	graceSeconds := int64(r.reuseGrace.Round(time.Second) / time.Second)
	// COALESCE keeps the first-use time, so retries cannot extend the
	// window. The connection reports matched rows (CLIENT_FOUND_ROWS), so
	// a retry that changes nothing still counts.
	res, err := tx.ExecContext(ctx, `UPDATE refresh_token SET used_at = COALESCE(used_at, UTC_TIMESTAMP())
		WHERE jti = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > UTC_TIMESTAMP()
		  AND (used_at IS NULL OR (? > 0 AND used_at >= UTC_TIMESTAMP() - INTERVAL ? SECOND))`,
		oldID, userID, graceSeconds, graceSeconds)
	if err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}

	if n == 1 {
		if err := insertRefreshToken(ctx, tx, newID, userID, newExpiresAt); err != nil {
			return fmt.Errorf("rotate refresh token: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("rotate refresh token: %w", err)
		}
		return nil
	}

	var exists bool
	if err := tx.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM refresh_token WHERE jti = ? AND user_id = ?)", oldID, userID).Scan(&exists); err != nil {
		return fmt.Errorf("rotate refresh token: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	if err := revokeAll(ctx, tx, userID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return ErrTokenReused
}

// RevokeAll revokes every outstanding refresh token of the user.
func (r *RefreshTokenRepository) RevokeAll(ctx context.Context, userID int64) error {
	return revokeAll(ctx, r.db, userID)
}

// Prune deletes up to 1000 expired tokens, oldest first.
func (r *RefreshTokenRepository) Prune(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM refresh_token WHERE expires_at < UTC_TIMESTAMP() ORDER BY expires_at LIMIT 1000")
	if err != nil {
		return fmt.Errorf("prune refresh tokens: %w", err)
	}
	return nil
}

func (r *RefreshTokenRepository) maybePrune(ctx context.Context) {
	r.pruneMu.Lock()
	due := time.Since(r.lastPrune) >= pruneInterval
	if due {
		r.lastPrune = time.Now()
	}
	r.pruneMu.Unlock()
	if !due {
		return
	}
	// Pruning is housekeeping: a failure must not block a login.
	if err := r.Prune(ctx); err != nil {
		slog.WarnContext(ctx, "refresh token prune failed", "error", err)
	}
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertRefreshToken(ctx context.Context, db execer, id string, userID int64, expiresAt time.Time) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO refresh_token (jti, user_id, expires_at) VALUES (?, ?, ?)",
		id, userID, expiresAt.UTC())
	return err
}

func revokeAll(ctx context.Context, db execer, userID int64) error {
	_, err := db.ExecContext(ctx,
		"UPDATE refresh_token SET revoked_at = UTC_TIMESTAMP() WHERE user_id = ? AND revoked_at IS NULL", userID)
	if err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}
