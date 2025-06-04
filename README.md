# AEMET Go Library

[![GoDoc](https://godoc.org/github.com/rubiojr/aemet-go?status.svg)](https://godoc.org/github.com/rubiojr/aemet-go)

A Go library for retrieving weather data from the Spanish Meteorological Agency (AEMET) API.

## Features

- Get weather station information
- Retrieve weather forecasts by municipality ID or name
- Built-in municipality search functionality
- Simple, lightweight client for accessing AEMET weather data
- Configurable via direct options or environment variables
- Returns structured Go objects for easy integration

## Installation

```bash
go get -u github.com/rubiojr/aemet-go
```

## Getting Started

### API Key

You need an AEMET API key to use this library. You can obtain one from the [AEMET OpenData website](https://opendata.aemet.es/centrodedescargas/obtencionAPIKey).

Set your API key as an environment variable:

```bash
export AEMET_API_KEY="your-api-key-here"
```

Or provide it directly in the configuration.

## Usage

### Basic Setup

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/rubiojr/aemet-go"
)

func main() {
    // Using environment variable AEMET_API_KEY
    client, err := aemet.NewWithDefaults()
    if err != nil {
        log.Fatal(err)
    }
    
    // Or configure directly
    client, err = aemet.New(aemet.Config{
        AemetApiKey: "your-api-key-here",
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

### Get Weather Stations

```go
stations, err := client.GetStations()
if err != nil {
    log.Fatal(err)
}

for _, station := range stations {
    fmt.Printf("Station: %s (%s)\n", station.Name, station.ID)
    fmt.Printf("Location: %s, %s\n", station.Latitude, station.Longitude)
    fmt.Printf("Province: %s\n", station.Province)
    fmt.Println()
}
```

### Get Weather Forecast by Municipality ID

```go
// Get forecast for Madrid (municipality ID: 28079)
forecast, err := client.GetForecastFor("28079")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Forecast for %s, %s\n", forecast.Nombre, forecast.Provincia)
fmt.Printf("Updated: %s\n", forecast.Elaborado)

for _, day := range forecast.Prediccion.Dia {
    fmt.Printf("Date: %s\n", day.Fecha)
    fmt.Printf("Max Temperature: %d°C\n", day.Temperatura.Maxima)
    fmt.Printf("Min Temperature: %d°C\n", day.Temperatura.Minima)
    
    if len(day.EstadoCielo) > 0 {
        fmt.Printf("Sky: %s\n", day.EstadoCielo[0].Descripcion)
    }
    
    if len(day.ProbPrecipitacion) > 0 {
        fmt.Printf("Precipitation Probability: %d%%\n", day.ProbPrecipitacion[0].Value)
    }
    
    fmt.Println()
}
```

### Get Weather Forecast by Municipality Name

```go
// Get forecast by municipality name
forecast, err := client.GetForecastByName("Madrid")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Forecast for %s\n", forecast.Nombre)
```

### Find Municipality Information

```go
// Find municipality ID by exact name
id, err := aemet.FindMunicipalityID("Madrid")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Madrid ID: %s\n", id)

// Search municipalities by partial name
municipalities, err := aemet.FindMunicipalitiesByPartialName("Barce")
if err != nil {
    log.Fatal(err)
}

for _, muni := range municipalities {
    fmt.Printf("%s (ID: %s)\n", muni.Name, muni.ID)
}

// Get municipality info by ID
info, err := aemet.GetMunicipalityByID("08019")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Municipality: %s\n", info.Name)
fmt.Printf("Population: %s\n", info.NumHab)
fmt.Printf("Coordinates: %s, %s\n", info.LatitudeDec, info.LongitudeDec)
```

## Configuration Options

The `Config` struct supports the following options:

```go
type Config struct {
    AemetApiKey             string        // AEMET API key
    AemetWeatherStationCode string        // Weather station code (currently unused)
    HTTPClient              *http.Client  // Custom HTTP client
    Logger                  *log.Logger   // Custom logger
}
```

## Environment Variables

- `AEMET_API_KEY` - Your AEMET API key

## Data Structures

### WeatherStation

```go
type WeatherStation struct {
    Latitude  string `json:"latitud"`
    Province  string `json:"provincia"`
    Altitude  string `json:"altitud"`
    ID        string `json:"indicativo"`
    Name      string `json:"nombre"`
    IndSinop  string `json:"indsinop"`
    Longitude string `json:"longitud"`
}
```

### Municipality Forecast

The `Municipality` struct contains detailed forecast information including:

- `Nombre` - Municipality name
- `Provincia` - Province name
- `Prediccion` - Forecast data with daily predictions
- Each day includes:
  - Temperature (max/min/hourly)
  - Precipitation probability
  - Sky conditions
  - Wind information
  - Relative humidity
  - UV index

### MunicipalityInfo

```go
type MunicipalityInfo struct {
    Name         string `json:"nombre"`
    ID           string `json:"id"`
    Capital      string `json:"capital"`
    NumHab       string `json:"num_hab"`
    Altitude     string `json:"altitud"`
    LatitudeDec  string `json:"latitud_dec"`
    LongitudeDec string `json:"longitud_dec"`
    // ... other fields
}
```

## Examples

See the `examples/` directory for more detailed usage examples:

- `municipality_search.go` - Search and find municipality information

## Error Handling

All methods return descriptive errors. Common error scenarios:

- Missing or invalid API key
- Municipality not found
- Network connectivity issues
- API rate limiting

## License

MIT