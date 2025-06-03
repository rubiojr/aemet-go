package aemet

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	aemetApi   = "https://opendata.aemet.es/opendata"
	retryCount = 5

	EnvAemetApiKey = "AEMET_API_KEY"
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
	err := c.getRedir(fmt.Sprintf("api/prediccion/especifica/municipio/diaria/%s", muni), &m)
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
