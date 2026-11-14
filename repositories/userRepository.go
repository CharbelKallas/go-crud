package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"

	"go-crud/models"
)

// mysqlErrDuplicateEntry is MySQL error ER_DUP_ENTRY.
const mysqlErrDuplicateEntry = 1062

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create stores the user. It returns ErrDuplicateEmail when the email is
// already taken (enforced by a unique index).
func (r *UserRepository) Create(ctx context.Context, user models.User) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO user (name, email, password) VALUES (?, ?, ?)", user.Name, user.Email, user.Password)
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == mysqlErrDuplicateEntry {
		return ErrDuplicateEmail
	}
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) UpdatePasswordHash(ctx context.Context, id int64, hash string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE user SET password = ? WHERE id = ?", hash, id)
	if err != nil {
		return fmt.Errorf("update password hash: %w", err)
	}
	return nil
}

// TokensRevokedAt returns when the user's sessions were last revoked (zero
// if never), or ErrNotFound if the user no longer exists.
func (r *UserRepository) TokensRevokedAt(ctx context.Context, email string) (time.Time, error) {
	var revoked sql.NullTime
	err := r.db.QueryRowContext(ctx, "SELECT tokens_revoked_at FROM user WHERE email = ?", email).Scan(&revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("tokens revoked at: %w", err)
	}
	return revoked.Time, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (models.User, error) {
	var user models.User
	err := r.db.QueryRowContext(ctx, "SELECT id, name, email, password FROM user WHERE email = ?", email).
		Scan(&user.ID, &user.Name, &user.Email, &user.Password)
	if errors.Is(err, sql.ErrNoRows) {
		return user, ErrNotFound
	}
	if err != nil {
		return user, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}
