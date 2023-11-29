package services

import (
	"github.com/gin-gonic/gin"
	"go-crud/models"
	"go-crud/repositories"
	"net/http"
	"strconv"
)

func GetAlbums(c *gin.Context) {
	albums, err := repositories.Albums()

	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.IndentedJSON(http.StatusOK, albums)
}

func PostAlbums(c *gin.Context) {
	var newAlbum models.Album

	if err := c.BindJSON(&newAlbum); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	addAlbum, err := repositories.AddAlbum(newAlbum)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.IndentedJSON(http.StatusCreated, addAlbum)
}

func UpdateAlbum(c *gin.Context) {
	var updatedAlbum models.Album
	id := c.Param("id")

	result, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := c.BindJSON(&updatedAlbum); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	albumUpdate, err := repositories.AlbumUpdate(result, updatedAlbum)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.IndentedJSON(http.StatusCreated, albumUpdate)
}

func GetAlbumByID(c *gin.Context) {
	id := c.Param("id")

	result, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	albumByID, err := repositories.AlbumByID(result)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.IndentedJSON(http.StatusOK, albumByID)
}
func GetAlbumByArtist(c *gin.Context) {
	artist := c.Param("artist")

	albumsByArtist, err := repositories.AlbumsByArtist(artist)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.IndentedJSON(http.StatusOK, albumsByArtist)
}
func DeleteAlbum(c *gin.Context) {
	id := c.Param("id")

	result, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	deleteAlbum, err := repositories.DeleteAlbum(result)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.IndentedJSON(http.StatusOK, deleteAlbum)
}
