package services

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"go-crud/models"
	"io"
	"net/http"
)

func CurrentWeather(c *gin.Context) {
	lat := c.Param("lat")
	long := c.Param("long")

	response, err := http.Get("http://api.weatherapi.com/v1/current.json?key=4bdd3d979499405cbc5120932233011&q=" + lat + "," + long)

	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var responseObject models.WeatherResponse
	err = json.Unmarshal(responseData, &responseObject)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.IndentedJSON(http.StatusOK, responseObject)
}
