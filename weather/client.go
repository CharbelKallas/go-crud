// Package weather is a small client for the weatherapi.com current-weather
// endpoint.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go-crud/models"
)

// ErrUpstream is returned when the weather API cannot be reached or
// answers with an error. Its message never contains the API key.
var ErrUpstream = errors.New("weather API request failed")

// maxResponseBytes caps how much of the upstream response is read.
const maxResponseBytes = 1 << 20

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient builds a client. transport may be nil to use the default.
func NewClient(baseURL, apiKey string, transport http.RoundTripper) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// Current returns the current weather at the coordinates.
func (c *Client) Current(ctx context.Context, lat, lon float64) (models.WeatherResponse, error) {
	var out models.WeatherResponse

	q := url.Values{}
	q.Set("key", c.apiKey)
	q.Set("q", strconv.FormatFloat(lat, 'f', -1, 64)+","+strconv.FormatFloat(lon, 'f', -1, 64))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/current.json?"+q.Encode(), nil)
	if err != nil {
		return out, fmt.Errorf("%w: build request", ErrUpstream)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// *url.Error includes the full URL, and so the API key: keep
		// only the underlying cause.
		var uErr *url.Error
		if errors.As(err, &uErr) {
			err = uErr.Err
		}
		return out, fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, maxResponseBytes)
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}
	if err := json.NewDecoder(body).Decode(&out); err != nil {
		return out, fmt.Errorf("%w: decode response: %w", ErrUpstream, err)
	}
	return out, nil
}
