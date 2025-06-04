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

	EnvAemetApiKey = "AEMET_API_KEY"

	maxRetries    = 3
	baseBackoffMs = 100
)

type Config struct {
	AemetApiKey             string
	AemetWeatherStationCode string
	HTTPClient              *http.Client
	Logger                  *log.Logger
}

type Client struct {
	config     Config
	httpClient *http.Client
	logger     *log.Logger
}

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

func NewWithDefaults() (*Client, error) {
	return New(Config{})
}

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

func (c *Client) GetStations() ([]WeatherStation, error) {
	var stations []WeatherStation
	err := c.getRedir("api/valores/climatologicos/inventarioestaciones/todasestaciones", &stations)
	if err != nil {
		return nil, fmt.Errorf("error requesting data: %w", err)
	}

	return stations, nil
}

// api/prediccion/especifica/municipio/diaria/<municipio>
// GetForecastFor gets the weather forecast for a municipality by its ID
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

// GetForecastByName gets the weather forecast for a municipality by its name
func (c *Client) GetForecastByName(name string) (*Municipality, error) {
	// Find the municipality ID first
	id, err := FindMunicipalityID(name)
	if err != nil {
		return nil, err
	}

	// Use the ID to get the forecast
	return c.GetForecastFor(id)
}
