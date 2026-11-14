package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"go-crud/models"
	"go-crud/repositories"
)

type AlbumStore interface {
	List(ctx context.Context) ([]models.Album, error)
	ListByArtist(ctx context.Context, artist string) ([]models.Album, error)
	Get(ctx context.Context, id int64) (models.Album, error)
	Create(ctx context.Context, alb models.Album) (int64, error)
	Update(ctx context.Context, id int64, alb models.Album) error
	Delete(ctx context.Context, id int64) error
}

type AlbumHandler struct {
	albums AlbumStore
}

func NewAlbumHandler(albums AlbumStore) *AlbumHandler {
	return &AlbumHandler{albums: albums}
}

func (h *AlbumHandler) List(c *gin.Context) {
	albums, err := h.albums.List(c.Request.Context())
	if err != nil {
		respondInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, albums)
}

func (h *AlbumHandler) ListByArtist(c *gin.Context) {
	albums, err := h.albums.ListByArtist(c.Request.Context(), c.Param("artist"))
	if err != nil {
		respondInternal(c, err)
		return
	}
	c.JSON(http.StatusOK, albums)
}

func (h *AlbumHandler) Get(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	album, err := h.albums.Get(c.Request.Context(), id)
	if !h.checkErr(c, err) {
		return
	}
	c.JSON(http.StatusOK, album)
}

// Create responds 201 with the new album's id.
func (h *AlbumHandler) Create(c *gin.Context) {
	var album models.Album
	if !bindJSON(c, &album) {
		return
	}
	id, err := h.albums.Create(c.Request.Context(), album)
	if err != nil {
		respondInternal(c, err)
		return
	}
	c.JSON(http.StatusCreated, id)
}

// Update responds 200 with the album's id.
func (h *AlbumHandler) Update(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	var album models.Album
	if !bindJSON(c, &album) {
		return
	}
	if !h.checkErr(c, h.albums.Update(c.Request.Context(), id, album)) {
		return
	}
	c.JSON(http.StatusOK, id)
}

// Delete responds 200 with the deleted album's id.
func (h *AlbumHandler) Delete(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	if !h.checkErr(c, h.albums.Delete(c.Request.Context(), id)) {
		return
	}
	c.JSON(http.StatusOK, id)
}

// checkErr answers 404 or 500 for a non-nil error and reports whether the
// handler should continue.
func (h *AlbumHandler) checkErr(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, repositories.ErrNotFound):
		respondError(c, http.StatusNotFound, "album not found")
	default:
		respondInternal(c, err)
	}
	return false
}

func albumID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(c, http.StatusBadRequest, "id must be a positive integer")
		return 0, false
	}
	return id, true
}
