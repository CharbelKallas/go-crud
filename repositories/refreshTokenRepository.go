package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RefreshTokenRepository records issued refresh tokens so that each one can
// be used only once and a user's tokens can be revoked together.
type RefreshTokenRepository struct {
	db *sql.DB
}

func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

// Save records a newly issued token. It also deletes a batch of expired
// rows so the table does not grow without bound.
func (r *RefreshTokenRepository) Save(ctx context.Context, id string, userID int64, expiresAt time.Time) error {
	if _, err := r.db.ExecContext(ctx, "DELETE FROM refresh_token WHERE expires_at < UTC_TIMESTAMP() LIMIT 100"); err != nil {
		return fmt.Errorf("prune refresh tokens: %w", err)
	}
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO refresh_token (jti, user_id, expires_at) VALUES (?, ?, ?)",
		id, userID, expiresAt.UTC())
	if err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

// Consume marks the token as used. It returns ErrNotFound for an unknown
// token, and ErrTokenReused when the token was already used, revoked or
// expired; in that case every refresh token of the user is revoked,
// because a second use means the token was copied.
func (r *RefreshTokenRepository) Consume(ctx context.Context, id string, userID int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE refresh_token SET used_at = UTC_TIMESTAMP()
		WHERE jti = ? AND user_id = ? AND used_at IS NULL AND revoked_at IS NULL AND expires_at > UTC_TIMESTAMP()`,
		id, userID)
	if err != nil {
		return fmt.Errorf("consume refresh token: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("consume refresh token: %w", err)
	} else if n == 1 {
		return nil
	}

	var exists bool
	err = r.db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM refresh_token WHERE jti = ? AND user_id = ?)", id, userID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("consume refresh token: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	if err := r.RevokeAll(ctx, userID); err != nil {
		return err
	}
	return ErrTokenReused
}

// RevokeAll revokes every outstanding refresh token of the user.
func (r *RefreshTokenRepository) RevokeAll(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE refresh_token SET revoked_at = UTC_TIMESTAMP() WHERE user_id = ? AND revoked_at IS NULL", userID)
	if err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}
