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
	"github.com/urfave/cli/v3"
)

const (
	maxDays        = 7
	maxSearchLimit = 50
)

const instructions = `Weather forecasts for Spanish municipalities from AEMET, Spain's state
meteorological agency. Call get_forecast with a town name or a 5-digit INE
municipality id. If the name is ambiguous the error lists candidates with
their ids; retry with an id or a province. Use search_municipalities to
find ids. Sky descriptions are in Spanish; temperatures are °C and wind
speeds km/h. Only Spain is covered.`

// Command serves MCP over stdin/stdout.
var Command = &cli.Command{
	Name:  "mcp",
	Usage: "Run a Model Context Protocol server on stdio for AI agents",
	Description: `Serves AEMET weather tools over MCP using stdin/stdout. Configure it in an
agent as a stdio server, for example:

   {"command": "go", "args": ["run", "github.com/rubiojr/aemet-go/cmd/aemet@latest", "mcp"],
    "env": {"AEMET_API_KEY": "..."}}

Tools: get_forecast, search_municipalities.`,
	Action: func(ctx context.Context, cmd *cli.Command) error {
		key := util.APIKey(cmd)
		errOut := cmd.Root().ErrWriter
		if key == "" {
			fmt.Fprintln(errOut, "aemet mcp: AEMET_API_KEY is not set; get_forecast will fail until it is")
		}
		s := NewServer(lazyClient(key), slog.New(slog.NewTextHandler(errOut, &slog.HandlerOptions{Level: slog.LevelWarn})))
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

// NewServer returns an MCP server whose tools fetch forecasts through the
// client returned by newClient.
func NewServer(newClient func() (util.Forecaster, error), logger *slog.Logger) *mcp.Server {
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
