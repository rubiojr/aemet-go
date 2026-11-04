// Package mcpserver implements the mcp command: a Model Context Protocol
// server on stdio that lets agents look up Spanish weather forecasts.
package mcpserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rubiojr/aemet-go/cmd/aemet/util"
	"github.com/rubiojr/aemet-go/cmd/aemet/weather"
	"github.com/rubiojr/aemet-go/radar"
	"github.com/urfave/cli/v3"
)

const (
	maxDays        = 7
	maxSearchLimit = 50
)

const instructions = `Weather forecasts and live radar for Spanish municipalities from AEMET,
Spain's state meteorological agency. Call get_forecast with a town name or a
5-digit INE municipality id. If the name is ambiguous the error lists
candidates with their ids; retry with an id or a province. Use
search_municipalities to find ids. Sky descriptions are in Spanish;
temperatures are °C and wind speeds km/h. Only Spain is covered.

Forecasts are daily and cannot say whether it is raining now. For "is it
raining?", "is rain coming?" or "how hard is it raining?", call get_radar:
it reads AEMET's latest radar composite (updated every 10 minutes, no API key
needed) at the town and around it, and compares the last hour to say whether
rain is approaching. Report the composite's time and age. The radar does not
cover the Canary Islands.`

// Command serves MCP over stdin/stdout.
var Command = &cli.Command{
	Name:  "mcp",
	Usage: "Run a Model Context Protocol server on stdio for AI agents",
	Description: `Serves AEMET weather tools over MCP using stdin/stdout. Configure it in an
agent as a stdio server, for example:

   {"command": "go", "args": ["run", "github.com/rubiojr/aemet-go/cmd/aemet@latest", "mcp"],
    "env": {"AEMET_API_KEY": "..."}}

Tools: get_forecast, get_radar, search_municipalities.`,
	Action: func(ctx context.Context, cmd *cli.Command) error {
		key := util.APIKey(cmd)
		errOut := cmd.Root().ErrWriter
		if key == "" {
			fmt.Fprintln(errOut, "aemet mcp: AEMET_API_KEY is not set; get_forecast will fail until it is")
		}
		s := NewServer(lazyClient(key), &radar.Client{}, slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: slog.LevelWarn})))
		return s.Run(ctx, &mcp.StdioTransport{})
	},
}

// lazyClient builds the AEMET client on first use so the server starts,
// and search works, without an API key.
func lazyClient(key string) func() (util.Forecaster, error) {
	var (
		mu sync.Mutex
		c  util.Forecaster
	)
	return func() (util.Forecaster, error) {
		mu.Lock()
		defer mu.Unlock()
		if c != nil {
			return c, nil
		}
		if key == "" {
			return nil, fmt.Errorf("the AEMET_API_KEY environment variable is not set for the aemet MCP server; ask the user to configure it (free key: https://opendata.aemet.es/centrodedescargas/obtencionAPIKey)")
		}
		var err error
		c, err = util.NewClient(key)
		return c, util.Redact(err, key)
	}
}

// ForecastInput is the get_forecast argument set.
type ForecastInput struct {
	Location string `json:"location" jsonschema:"Spanish municipality name (e.g. Madrid, A Coruña, Vitoria-Gasteiz) or 5-digit INE id (e.g. 28079); case and accents are ignored"`
	Province string `json:"province,omitempty" jsonschema:"optional province to disambiguate names shared by several municipalities"`
	Days     int    `json:"days,omitempty" jsonschema:"number of days starting today, 1-7; default 7"`
}

// SearchInput is the search_municipalities argument set.
type SearchInput struct {
	Query    string `json:"query" jsonschema:"full or partial municipality name; case and accents are ignored"`
	Province string `json:"province,omitempty" jsonschema:"optional province filter"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum results, 1-50; default 10"`
}

// SearchOutput is the search_municipalities result.
type SearchOutput struct {
	Municipalities []weather.Place `json:"municipalities"`
}

// RadarInput is the get_radar argument set.
type RadarInput struct {
	Location       string  `json:"location" jsonschema:"Spanish municipality name or 5-digit INE id, as for get_forecast"`
	Province       string  `json:"province,omitempty" jsonschema:"optional province to disambiguate names shared by several municipalities"`
	RadiusKm       float64 `json:"radius_km,omitempty" jsonschema:"search radius around the town in km, 1-200; default 25"`
	HistoryMinutes int     `json:"history_minutes,omitempty" jsonschema:"minutes of earlier composites to compare for the trend, 10-180; default 60"`
}

// RadarOutput is the get_radar result.
type RadarOutput struct {
	Place    weather.Place `json:"place"`
	Lat      float64       `json:"lat"`
	Lon      float64       `json:"lon"`
	RadiusKm float64       `json:"radius_km"`
	Summary  string        `json:"summary" jsonschema:"one-line English summary of the report"`
	radar.Report
}

// NewServer returns an MCP server whose tools fetch forecasts through the
// client returned by newClient and radar images through rc.
func NewServer(newClient func() (util.Forecaster, error), rc *radar.Client, logger *slog.Logger) *mcp.Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := mcp.NewServer(&mcp.Implementation{
		Name:    util.AppName,
		Title:   "AEMET Spanish weather",
		Version: util.GetVersion(),
	}, &mcp.ServerOptions{Instructions: instructions, Logger: logger})

	openWorld, readOnly := true, true
	mcp.AddTool(s, &mcp.Tool{
		Name:  "get_forecast",
		Title: "Get weather forecast",
		Description: "Daily weather forecast for a municipality in Spain from AEMET: sky, " +
			"min/max and feels-like temperature (°C), rain probability (%), wind (km/h), " +
			"humidity and UV index, split into morning and afternoon where available. " +
			"Use days=1 for today only.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: true, OpenWorldHint: &openWorld},
	}, forecastHandler(newClient))

	mcp.AddTool(s, &mcp.Tool{
		Name:  "get_radar",
		Title: "Check weather radar",
		Description: "Is it raining now in a Spanish municipality? Reads AEMET's latest radar " +
			"composite (every 10 minutes) and reports precipitation at the town (dry, " +
			"precipitation or no_coverage, with dBZ, label and approximate mm/h), the nearest " +
			"and the strongest precipitation within radius_km with distance and compass " +
			"direction, the share of the area with precipitation, and a trend over the last " +
			"history_minutes (approaching with speed and ETA, receding, arrived, cleared...). " +
			"Includes a one-line summary. Mainland Spain and the Balearic Islands only. " +
			"Needs no API key.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, OpenWorldHint: &openWorld},
	}, radarHandler(rc))

	closedWorld := false
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_municipalities",
		Title:       "Search Spanish municipalities",
		Description: "Find Spanish municipalities by name and get their 5-digit INE ids and provinces, best matches first. Works offline.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, searchHandler)
	return s
}

func forecastHandler(newClient func() (util.Forecaster, error)) mcp.ToolHandlerFor[ForecastInput, weather.Forecast] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in ForecastInput) (*mcp.CallToolResult, weather.Forecast, error) {
		days := in.Days
		if days == 0 {
			days = maxDays
		}
		if days < 1 || days > maxDays {
			return nil, weather.Forecast{}, fmt.Errorf("days must be between 1 and %d", maxDays)
		}
		// Resolve before needing a client so name errors don't depend on the key.
		place, err := weather.Resolve(in.Location, in.Province)
		if err != nil {
			return nil, weather.Forecast{}, err
		}
		c, err := newClient()
		if err != nil {
			return nil, weather.Forecast{}, err
		}
		f, err := weather.Fetch(c, place.ID, "", days)
		if err != nil {
			return nil, weather.Forecast{}, fmt.Errorf("AEMET request failed, try again later: %w", err)
		}
		f.Place = place
		return nil, f, nil
	}
}

func searchHandler(_ context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
	limit := in.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > maxSearchLimit {
		return nil, SearchOutput{}, fmt.Errorf("limit must be between 1 and %d", maxSearchLimit)
	}
	places, err := weather.Search(in.Query, in.Province, limit)
	if err != nil {
		return nil, SearchOutput{}, err
	}
	if places == nil {
		places = []weather.Place{}
	}
	return nil, SearchOutput{Municipalities: places}, nil
}

func radarHandler(rc *radar.Client) mcp.ToolHandlerFor[RadarInput, RadarOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in RadarInput) (*mcp.CallToolResult, RadarOutput, error) {
		opt, err := radarOptions(in)
		if err != nil {
			return nil, RadarOutput{}, err
		}
		place, err := weather.Resolve(in.Location, in.Province)
		if err != nil {
			return nil, RadarOutput{}, err
		}
		lat, lon, err := weather.Coordinates(place.ID)
		if err != nil {
			return nil, RadarOutput{}, err
		}
		r, err := rc.Check(ctx, lat, lon, opt)
		if err != nil {
			return nil, RadarOutput{}, fmt.Errorf("AEMET radar request failed, try again later: %w", err)
		}
		// Per-frame details stay out of the result to keep it small; the
		// trend already summarises them.
		r.History = slimHistory(r.History)
		return nil, RadarOutput{
			Place: place, Lat: lat, Lon: lon, RadiusKm: opt.RadiusKm,
			Summary: weather.RadarSummary(place.Name, opt.RadiusKm, r),
			Report:  r,
		}, nil
	}
}

// radarOptions validates the get_radar arguments and applies defaults.
func radarOptions(in RadarInput) (radar.Options, error) {
	opt := radar.Options{RadiusKm: in.RadiusKm, HistoryMinutes: in.HistoryMinutes}
	if opt.RadiusKm == 0 {
		opt.RadiusKm = radar.DefaultRadiusKm
	}
	if opt.HistoryMinutes == 0 {
		opt.HistoryMinutes = radar.DefaultHistoryMinutes
	}
	if opt.RadiusKm < 1 || opt.RadiusKm > radar.MaxRadiusKm {
		return opt, fmt.Errorf("radius_km must be between 1 and %d", radar.MaxRadiusKm)
	}
	if opt.HistoryMinutes < 10 || opt.HistoryMinutes > radar.MaxHistoryMinutes {
		return opt, fmt.Errorf("history_minutes must be between 10 and %d", radar.MaxHistoryMinutes)
	}
	return opt, nil
}

// slimHistory drops the nearest and strongest echoes of earlier frames.
func slimHistory(h []radar.Snapshot) []radar.Snapshot {
	for i := range h {
		h[i].Nearest, h[i].Strongest = nil, nil
	}
	return h
}
