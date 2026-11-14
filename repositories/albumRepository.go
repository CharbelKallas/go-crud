package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go-crud/models"
)

const albumColumns = "id, title, artist, price"

type AlbumRepository struct {
	db *sql.DB
}

func NewAlbumRepository(db *sql.DB) *AlbumRepository {
	return &AlbumRepository{db: db}
}

// List returns every album, ordered by id. It never returns a nil slice so
// an empty result is rendered as [] rather than null.
func (r *AlbumRepository) List(ctx context.Context) ([]models.Album, error) {
	return r.query(ctx, "SELECT "+albumColumns+" FROM album ORDER BY id")
}

func (r *AlbumRepository) ListByArtist(ctx context.Context, artist string) ([]models.Album, error) {
	return r.query(ctx, "SELECT "+albumColumns+" FROM album WHERE artist = ? ORDER BY id", artist)
}

func (r *AlbumRepository) Get(ctx context.Context, id int64) (models.Album, error) {
	var alb models.Album
	err := r.db.QueryRowContext(ctx, "SELECT "+albumColumns+" FROM album WHERE id = ?", id).
		Scan(&alb.ID, &alb.Title, &alb.Artist, &alb.Price)
	if errors.Is(err, sql.ErrNoRows) {
		return alb, ErrNotFound
	}
	if err != nil {
		return alb, fmt.Errorf("get album %d: %w", id, err)
	}
	return alb, nil
}

func (r *AlbumRepository) Create(ctx context.Context, alb models.Album) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO album (title, artist, price) VALUES (?, ?, ?)", alb.Title, alb.Artist, alb.Price)
	if err != nil {
		return 0, fmt.Errorf("create album: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("create album: %w", err)
	}
	return id, nil
}

// Update replaces the album's fields. It returns ErrNotFound when no album
// has the id. The connection uses CLIENT_FOUND_ROWS, so an update that
// changes nothing still counts the matched row.
func (r *AlbumRepository) Update(ctx context.Context, id int64, alb models.Album) error {
	res, err := r.db.ExecContext(ctx, "UPDATE album SET title = ?, artist = ?, price = ? WHERE id = ?", alb.Title, alb.Artist, alb.Price, id)
	if err != nil {
		return fmt.Errorf("update album %d: %w", id, err)
	}
	return expectOneRow(res, "update album", id)
}

func (r *AlbumRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM album WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete album %d: %w", id, err)
	}
	return expectOneRow(res, "delete album", id)
}

func (r *AlbumRepository) query(ctx context.Context, q string, args ...any) ([]models.Album, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list albums: %w", err)
	}
	defer rows.Close()

	albums := []models.Album{}
	for rows.Next() {
		var alb models.Album
		if err := rows.Scan(&alb.ID, &alb.Title, &alb.Artist, &alb.Price); err != nil {
			return nil, fmt.Errorf("list albums: %w", err)
		}
		albums = append(albums, alb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list albums: %w", err)
	}
	return albums, nil
}

func expectOneRow(res sql.Result, op string, id int64) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s %d: %w", op, id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
