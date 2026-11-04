// Package radar implements the radar command.
package radar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rubiojr/aemet-go/cmd/aemet/weather"
	"github.com/rubiojr/aemet-go/radar"
	"github.com/urfave/cli/v3"
)

// Client is the radar client used by the command. Tests replace it.
var Client = &radar.Client{}

// Command reports what AEMET's weather radar shows around a municipality.
var Command = &cli.Command{
	Name:      "radar",
	Aliases:   []string{"r"},
	Usage:     "Is it raining now? Check the latest radar around a municipality (no API key needed)",
	ArgsUsage: "<municipality name or INE id>",
	Description: `Reads AEMET's latest national radar composite, published every 10
minutes, and reports precipitation at the municipality and within --radius
km of it, with intensity, the nearest and strongest echoes, and whether rain
is approaching over the last --history minutes. Covers mainland Spain and
the Balearic Islands, not the Canary Islands. Ambiguous names use the best
match.

Examples:
   aemet radar "El Masnou"
   aemet radar --radius 50 --json 08118`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "province",
			Aliases: []string{"p"},
			Usage:   "only match municipalities in this province",
		},
		&cli.FloatFlag{
			Name:  "radius",
			Value: radar.DefaultRadiusKm,
			Usage: fmt.Sprintf("search radius in km (max %d)", radar.MaxRadiusKm),
		},
		&cli.IntFlag{
			Name:  "history",
			Value: radar.DefaultHistoryMinutes,
			Usage: fmt.Sprintf("minutes of earlier composites for the trend (0 to %d)", radar.MaxHistoryMinutes),
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "print the full report as JSON",
		},
	},
	Action: run,
}

// Output is the JSON printed by the radar command.
type Output struct {
	Place    weather.Place `json:"place"`
	Lat      float64       `json:"lat"`
	Lon      float64       `json:"lon"`
	RadiusKm float64       `json:"radius_km"`
	Summary  string        `json:"summary"`
	radar.Report
}

func run(ctx context.Context, cmd *cli.Command) error {
	query := strings.TrimSpace(strings.Join(cmd.Args().Slice(), " "))
	if query == "" {
		return fmt.Errorf("municipality name or id required, e.g. aemet radar Madrid")
	}
	radius := cmd.Float("radius")
	if radius <= 0 {
		return fmt.Errorf("--radius must be positive")
	}
	history := cmd.Int("history")
	place, err := weather.Resolve(query, cmd.String("province"))
	var amb *weather.AmbiguousError
	if errors.As(err, &amb) {
		fmt.Fprintf(cmd.Root().ErrWriter, "%d municipalities match %q, using %s\n", len(amb.Candidates), query, amb.Candidates[0])
		place, err = amb.Candidates[0], nil
	}
	if err != nil {
		return err
	}
	lat, lon, err := weather.Coordinates(place.ID)
	if err != nil {
		return err
	}
	r, err := Client.Check(ctx, lat, lon, radar.Options{RadiusKm: radius, HistoryMinutes: history, NoHistory: history == 0})
	if err != nil {
		return err
	}
	summary := weather.RadarSummary(place.Name, radius, r)
	out := cmd.Root().Writer
	if cmd.Bool("json") {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(Output{Place: place, Lat: lat, Lon: lon, RadiusKm: radius, Summary: summary, Report: r})
	}
	_, err = fmt.Fprintln(out, summary)
	return err
}
