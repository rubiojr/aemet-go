// Package aemet provides a client for accessing the Spanish State Meteorological Agency (AEMET) OpenData API.
//
// This package allows you to retrieve weather forecasts, weather station data, and other meteorological
// information from the official AEMET API service.
//
// Basic usage:
//
//	client, err := aemet.NewWithDefaults()
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	forecast, err := client.GetForecastByName("Madrid")
//	if err != nil {
//		log.Fatal(err)
//	}
//
// The client requires an API key which can be obtained from https://opendata.aemet.es/centrodedescargas/obtencionAPIKey
// The API key can be set via the Config struct or the AEMET_API_KEY environment variable.
package aemet

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"
)

const (
	aemetApi   = "https://opendata.aemet.es/opendata"
	retryCount = 5

	// EnvAemetApiKey is the environment variable name for the AEMET API key
	EnvAemetApiKey = "AEMET_API_KEY"

	maxRetries    = 3
	baseBackoffMs = 100
)

// Config holds the configuration for the AEMET client.
type Config struct {
	// AemetApiKey is the API key for accessing AEMET services.
	// If empty, the client will attempt to read from the AEMET_API_KEY environment variable.
	AemetApiKey string

	// AemetWeatherStationCode specifies a default weather station code for requests.
	// This field is currently unused but reserved for future functionality.
	AemetWeatherStationCode string

	// HTTPClient allows customization of the HTTP client used for requests.
	// If nil, a default client with 30-second timeout will be used.
	HTTPClient *http.Client

	// Logger specifies a custom logger for the client.
	// If nil, a default logger writing to stderr will be used.
	Logger *log.Logger
}

// Client provides access to the AEMET OpenData API.
type Client struct {
	config     Config
	httpClient *http.Client
	logger     *log.Logger
}

// New creates a new AEMET client with the provided configuration.
// If no API key is provided in the config, it will attempt to read from the AEMET_API_KEY environment variable.
// Returns an error if no API key can be found.
func New(config Config) (*Client, error) {
	if config.AemetApiKey == "" {
		apiKey := os.Getenv(EnvAemetApiKey)
		if apiKey == "" {
			return nil, fmt.Errorf("AemetApiKey is required (set via Config or %s environment variable)", EnvAemetApiKey)
		}
		config.AemetApiKey = apiKey
	}

	client := &Client{
		config: config,
		logger: config.Logger,
	}

	if client.logger == nil {
		client.logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	if config.HTTPClient != nil {
		client.httpClient = config.HTTPClient
	} else {
		client.httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return client, nil
}

// NewWithDefaults creates a new AEMET client with default configuration.
// The API key will be read from the AEMET_API_KEY environment variable.
// Returns an error if the environment variable is not set.
func NewWithDefaults() (*Client, error) {
	return New(Config{})
}

// getRedir performs a two-step request to the AEMET API.
// Many AEMET endpoints return a redirect URL that must be followed to get the actual data.
func (c *Client) getRedir(path string, t any) error {
	r, err := c.httpClient.Get(fmt.Sprintf("%s/%s?api_key=%s", aemetApi, path, c.config.AemetApiKey))
	if err != nil {
		return fmt.Errorf("error requesting data: %w", err)
	}
	defer r.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		return fmt.Errorf("error decoding data: %w", err)
	}

	r, err = c.httpClient.Get(fmt.Sprintf("%s?api_key=%s", data["datos"], c.config.AemetApiKey))
	if err != nil {
		return fmt.Errorf("error requesting data: %w", err)
	}
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(t); err != nil {
		return fmt.Errorf("error decoding data: %w", err)
	}

	return nil
}

// getRedirWithRetry performs a two-step request with exponential backoff retry logic.
// This is useful for handling temporary network issues or API rate limits.
func (c *Client) getRedirWithRetry(path string, t any) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoffMs := baseBackoffMs * int(math.Pow(2, float64(attempt-1)))
			c.logger.Printf("Retrying request (attempt %d/%d) after %dms backoff", attempt+1, maxRetries+1, backoffMs)
			time.Sleep(time.Duration(backoffMs) * time.Millisecond)
		}

		err := c.getRedir(path, t)
		if err == nil {
			return nil
		}

		lastErr = err
		c.logger.Printf("Request failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
	}

	return fmt.Errorf("request failed after %d attempts: %w", maxRetries+1, lastErr)
}

// GetStations retrieves a list of all weather stations available in the AEMET network.
// Returns a slice of WeatherStation structs containing station metadata such as
// location, altitude, and identification codes.
func (c *Client) GetStations() ([]WeatherStation, error) {
	var stations []WeatherStation
	err := c.getRedir("api/valores/climatologicos/inventarioestaciones/todasestaciones", &stations)
	if err != nil {
		return nil, fmt.Errorf("error requesting data: %w", err)
	}

	return stations, nil
}

// GetForecastFor retrieves the daily weather forecast for a municipality using its official ID.
// The municipality ID should be the official INE (National Statistics Institute) code.
// Returns detailed forecast information including temperature, precipitation, wind, and other meteorological data.
func (c *Client) GetForecastFor(muni string) (*Municipality, error) {
	var m []*Municipality
	err := c.getRedirWithRetry(fmt.Sprintf("api/prediccion/especifica/municipio/diaria/%s", muni), &m)
	if err != nil {
		return nil, fmt.Errorf("error requesting data: %w", err)
	}

	if len(m) == 0 {
		return nil, fmt.Errorf("no data found for municipality %s", muni)
	}

	return m[0], nil
}

// GetForecastByName retrieves the daily weather forecast for a municipality using its name.
// The function first resolves the municipality name to its official ID, then fetches the forecast.
// This is a convenience method that combines municipality lookup with forecast retrieval.
func (c *Client) GetForecastByName(name string) (*Municipality, error) {
	id, err := FindMunicipalityID(name)
	if err != nil {
		return nil, err
	}

	return c.GetForecastFor(id)
}