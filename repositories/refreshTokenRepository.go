package repositories

import (
	"context"
	"database/sql"
	"errors"
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
	// presented again (whole seconds; config.Load enforces this). It lets
	// a client retry a refresh whose response it never received without
	// triggering theft detection. Zero makes tokens strictly single-use.
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

// Rotated is the refresh token the client should receive after Rotate.
type Rotated struct {
	ID        string
	ExpiresAt time.Time
}

// Rotate exchanges the refresh token oldID for a new one in a single
// transaction: either the old token is marked used and newID stored, or
// nothing changes, so a failed refresh never burns the client's token.
//
// A retry within the grace window (the client never received the response)
// gets back the token that was already issued for oldID, not a new one, so
// a replayed token cannot start an independent chain: the original client
// and the replayer then hold the same token, and whichever presents it
// second triggers reuse detection.
//
// It returns ErrNotFound for an unknown token. For a token that was
// already used (outside the grace window), revoked or expired it revokes
// every refresh token of the user and returns ErrTokenReused.
func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldID string, userID int64, newID string, newExpiresAt time.Time) (Rotated, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Rotated{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	defer tx.Rollback()

	graceSeconds := int64(r.reuseGrace / time.Second)
	var (
		used, revoked, live, inGrace bool
		replacedBy                   sql.NullString
	)
	// Time comparisons run in the database so app/DB clock skew does not
	// matter. FOR UPDATE serialises concurrent rotations of the same token.
	err = tx.QueryRowContext(ctx, `SELECT used_at IS NOT NULL, revoked_at IS NOT NULL,
			expires_at > UTC_TIMESTAMP(),
			COALESCE(? > 0 AND used_at >= UTC_TIMESTAMP() - INTERVAL ? SECOND, FALSE),
			replaced_by
		FROM refresh_token WHERE jti = ? AND user_id = ? FOR UPDATE`,
		graceSeconds, graceSeconds, oldID, userID).Scan(&used, &revoked, &live, &inGrace, &replacedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return Rotated{}, ErrNotFound
	}
	if err != nil {
		return Rotated{}, fmt.Errorf("rotate refresh token: %w", err)
	}

	switch {
	case !used && !revoked && live:
		if _, err := tx.ExecContext(ctx,
			"UPDATE refresh_token SET used_at = UTC_TIMESTAMP(), replaced_by = ? WHERE jti = ?", newID, oldID); err != nil {
			return Rotated{}, fmt.Errorf("rotate refresh token: %w", err)
		}
		if err := insertRefreshToken(ctx, tx, newID, userID, newExpiresAt); err != nil {
			return Rotated{}, fmt.Errorf("rotate refresh token: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return Rotated{}, fmt.Errorf("rotate refresh token: %w", err)
		}
		return Rotated{ID: newID, ExpiresAt: newExpiresAt}, nil

	case used && !revoked && live && inGrace && replacedBy.Valid:
		// Retry: hand back the child, provided it is still unused.
		var child Rotated
		var childUsable bool
		err := tx.QueryRowContext(ctx, `SELECT jti, expires_at,
				used_at IS NULL AND revoked_at IS NULL AND expires_at > UTC_TIMESTAMP()
			FROM refresh_token WHERE jti = ? AND user_id = ? FOR UPDATE`,
			replacedBy.String, userID).Scan(&child.ID, &child.ExpiresAt, &childUsable)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Rotated{}, fmt.Errorf("rotate refresh token: %w", err)
		}
		if err == nil && childUsable {
			if err := tx.Commit(); err != nil {
				return Rotated{}, fmt.Errorf("rotate refresh token: %w", err)
			}
			return child, nil
		}
	}

	if err := revokeAll(ctx, tx, userID); err != nil {
		return Rotated{}, err
	}
	if err := tx.Commit(); err != nil {
		return Rotated{}, fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return Rotated{}, ErrTokenReused
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
