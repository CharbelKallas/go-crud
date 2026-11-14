package handlers

import (
	"context"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"go-crud/models"
)

type WeatherProvider interface {
	Current(ctx context.Context, lat, lon float64) (models.WeatherResponse, error)
}

type WeatherHandler struct {
	provider WeatherProvider
}

// NewWeatherHandler builds the handler. provider may be nil when no API key
// is configured; the endpoint then answers 503.
func NewWeatherHandler(provider WeatherProvider) *WeatherHandler {
	return &WeatherHandler{provider: provider}
}

func (h *WeatherHandler) Current(c *gin.Context) {
	lat, okLat := parseCoordinate(c.Param("lat"), 90)
	lon, okLon := parseCoordinate(c.Param("long"), 180)
	if !okLat || !okLon {
		respondError(c, http.StatusBadRequest, "lat must be a number in [-90, 90] and long a number in [-180, 180]")
		return
	}
	if h.provider == nil {
		respondError(c, http.StatusServiceUnavailable, "weather service is not configured")
		return
	}

	weather, err := h.provider.Current(c.Request.Context(), lat, lon)
	if err != nil {
		logErr(c, err)
		respondError(c, http.StatusBadGateway, "weather service unavailable")
		return
	}
	c.JSON(http.StatusOK, weather)
}

func parseCoordinate(s string, limit float64) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < -limit || v > limit {
		return 0, false
	}
	return v, true
}
