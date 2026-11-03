// Package day implements the day command.
package day

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/rubiojr/aemet-go/cmd/aemet/util"
	"github.com/rubiojr/aemet-go/cmd/aemet/weather"
	"github.com/urfave/cli/v3"
)

// Command prints a one-line summary of today's weather for several places.
var Command = &cli.Command{
	Name:      "day",
	Aliases:   []string{"d"},
	Usage:     "Show today's weather summary for one or more municipalities",
	ArgsUsage: "[municipality ...]",
	Description: `Prints one line per municipality with today's sky, temperature range,
rain probability and wind. Ambiguous names use the best match: exact names
first, then the most populated town.

Examples:
   aemet day Madrid Barcelona Sevilla
   aemet day -c "A Coruña" -c 28079`,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "cities",
			Aliases: []string{"c"},
			Usage:   "municipality names or INE ids (repeatable, or comma separated)",
		},
		&cli.BoolFlag{
			Name:    "use-ids",
			Aliases: []string{"ids"},
			Usage:   "require every city to be a 5-digit INE municipality id",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "print today's forecast for each city as JSON",
		},
	},
	Action: run,
}

func run(ctx context.Context, cmd *cli.Command) error {
	cities := append(cmd.StringSlice("cities"), cmd.Args().Slice()...)
	if len(cities) == 0 {
		return fmt.Errorf("at least one municipality is required, e.g. aemet day Madrid Bilbao")
	}
	client, err := util.Client(cmd)
	if err != nil {
		return err
	}
	out, errOut := cmd.Root().Writer, cmd.Root().ErrWriter
	useIDs, asJSON := cmd.Bool("use-ids"), cmd.Bool("json")

	var forecasts []weather.Forecast
	var lines []string
	failed := 0
	for _, city := range cities {
		f, line, err := todayLine(client, city, useIDs)
		if err != nil {
			failed++
			fmt.Fprintf(errOut, "❌ %s: %v\n", city, util.Redact(err, util.APIKey(cmd)))
			continue
		}
		forecasts, lines = append(forecasts, f), append(lines, line)
	}
	if err := write(out, asJSON, forecasts, lines); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d municipalities failed", failed, len(cities))
	}
	return nil
}

func write(w io.Writer, asJSON bool, forecasts []weather.Forecast, lines []string) error {
	if asJSON {
		if forecasts == nil {
			forecasts = []weather.Forecast{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(forecasts)
	}
	_, err := fmt.Fprintf(w, "🌤️  El tiempo hoy\n==============================================\n%s",
		strings.Join(append(lines, ""), "\n"))
	return err
}

// todayLine fetches today's forecast for city and its one-line summary.
func todayLine(client util.Forecaster, city string, idsOnly bool) (weather.Forecast, string, error) {
	f, err := today(client, city, idsOnly)
	if err != nil {
		return f, "", err
	}
	line, err := weather.Summary(f)
	return f, line, err
}

func today(client util.Forecaster, city string, idsOnly bool) (weather.Forecast, error) {
	city = strings.TrimSpace(city)
	if idsOnly && !weather.IsID(city) {
		return weather.Forecast{}, fmt.Errorf("not a 5-digit INE municipality id")
	}
	place, err := weather.Resolve(city, "")
	var amb *weather.AmbiguousError
	if errors.As(err, &amb) {
		place, err = amb.Candidates[0], nil
	}
	if err != nil {
		return weather.Forecast{}, err
	}
	f, err := weather.Fetch(client, place.ID, "", 1)
	f.Place = place
	return f, err
}
