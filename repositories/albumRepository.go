package repositories

import (
	"database/sql"
	"errors"
	"fmt"
	"go-crud/models"
)

func Albums() ([]models.Album, error) {
	var albums []models.Album

	rows, err := Db.Query("SELECT * FROM album")
	if err != nil {
		return nil, fmt.Errorf("albums: %v", err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {

		}
	}(rows)

	for rows.Next() {
		var alb models.Album
		if err := rows.Scan(&alb.ID, &alb.Title, &alb.Artist, &alb.Price); err != nil {
			return nil, fmt.Errorf("albums: %v", err)
		}
		albums = append(albums, alb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("albums: %v", err)
	}
	return albums, nil
}
func AlbumsByArtist(name string) ([]models.Album, error) {
	var albums []models.Album

	rows, err := Db.Query("SELECT * FROM album WHERE artist = ?", name)
	if err != nil {
		return nil, fmt.Errorf("albumsByArtist %q: %v", name, err)
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {

		}
	}(rows)

	for rows.Next() {
		var alb models.Album
		if err := rows.Scan(&alb.ID, &alb.Title, &alb.Artist, &alb.Price); err != nil {
			return nil, fmt.Errorf("albumsByArtist %q: %v", name, err)
		}
		albums = append(albums, alb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("albumsByArtist %q: %v", name, err)
	}
	return albums, nil
}

func AlbumByID(id int64) (models.Album, error) {
	var alb models.Album

	row := Db.QueryRow("SELECT * FROM album WHERE id = ?", id)
	if err := row.Scan(&alb.ID, &alb.Title, &alb.Artist, &alb.Price); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return alb, fmt.Errorf("albumsById %d: no such album", id)
		}
		return alb, fmt.Errorf("albumsById %d: %v", id, err)
	}
	return alb, nil
}

func AddAlbum(alb models.Album) (int64, error) {
	result, err := Db.Exec("INSERT INTO album (title, artist, price) VALUES (?, ?, ?)", alb.Title, alb.Artist, alb.Price)
	if err != nil {
		return 0, fmt.Errorf("addAlbum: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("addAlbum: %v", err)
	}
	return id, nil
}

func AlbumUpdate(id int64, alb models.Album) (int64, error) {
	_, err := Db.Exec("UPDATE album set title = ?, artist = ?, price = ? where id = ?", alb.Title, alb.Artist, alb.Price, id)

	if err != nil {
		return 0, fmt.Errorf("addAlbum: %v", err)
	}

	return id, nil
}
func DeleteAlbum(id int64) (int64, error) {
	_, err := Db.Exec("DELETE FROM album WHERE id = ?", id)

	if err != nil {
		return 0, fmt.Errorf("DeleteAlbum: %v", err)
	}

	return id, nil
}
