package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"go-crud/models"
	"io"
	"net/http"
)

type APIKeyInterceptor struct {
	APISecret   string
	APIClientId string
	Transport   http.RoundTripper
}

func (i *APIKeyInterceptor) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		h := hmac.New(sha256.New, []byte(i.APISecret))
		h.Write(body)
		req.Body = io.NopCloser(bytes.NewBuffer(body))
		signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

		req.Header.Set("signature", signature)
	}

	req.Header.Set("client-id", i.APIClientId)
	req.Header.Set("Content-Type", "application/json")
	return i.Transport.RoundTrip(req)
}
func CurrentWeather(c *gin.Context) {
	lat := c.Param("lat")
	long := c.Param("long")

	apiSecret := "apiSecret"
	apiClientId := "apiClientId"

	client := &http.Client{
		Transport: &APIKeyInterceptor{
			APISecret:   apiSecret,
			APIClientId: apiClientId,
			Transport:   http.DefaultTransport,
		},
	}

	url := fmt.Sprintf("%s?key=%s&q=%s,%s", "http://api.weatherapi.com/v1/current.json", "4bdd3d979499405cbc5120932233011", lat, long)

	location := struct {
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	}{
		Latitude:  lat,
		Longitude: long,
	}

	jsonData, err := json.Marshal(location)
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return
	}

	response, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))

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
