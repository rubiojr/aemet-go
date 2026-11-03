// Package forecast implements the forecast command.
package forecast

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/rubiojr/aemet-go/cmd/aemet/util"
	"github.com/rubiojr/aemet-go/cmd/aemet/weather"
	"github.com/urfave/cli/v3"
)

// Command prints a multi-day forecast for one municipality.
var Command = &cli.Command{
	Name:      "forecast",
	Aliases:   []string{"f"},
	Usage:     "Show the daily forecast for a municipality",
	ArgsUsage: "<municipality name or INE id>",
	Description: `Shows AEMET's daily forecast (up to 7 days) for a Spanish municipality.
Names match ignoring case and accents. When several municipalities match,
you are asked to pick one; use --province to narrow the search or
--non-interactive to take the best match.

Examples:
   aemet forecast Madrid
   aemet forecast --province Asturias Mieres
   aemet forecast --json --days 2 28079`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "name",
			Aliases: []string{"n"},
			Usage:   "municipality name or INE id (alternative to the argument)",
		},
		&cli.StringFlag{
			Name:    "province",
			Aliases: []string{"p"},
			Usage:   "only match municipalities in this province",
		},
		&cli.IntFlag{
			Name:  "days",
			Usage: "number of days to show (0 for all)",
		},
		&cli.BoolFlag{
			Name:    "non-interactive",
			Aliases: []string{"i"},
			Usage:   "use the best match instead of asking when several municipalities match",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "print the forecast as JSON",
		},
	},
	Action: run,
}

func run(ctx context.Context, cmd *cli.Command) error {
	query, err := queryArg(cmd)
	if err != nil {
		return err
	}
	if cmd.Int("days") < 0 {
		return fmt.Errorf("--days must not be negative")
	}
	client, err := util.Client(cmd)
	if err != nil {
		return err
	}
	place, err := choose(cmd, query)
	if err != nil {
		return err
	}
	f, err := weather.Fetch(client, place.ID, "", cmd.Int("days"))
	if err != nil {
		return util.Redact(err, util.APIKey(cmd))
	}
	f.Place = place
	if cmd.Bool("json") {
		enc := json.NewEncoder(cmd.Root().Writer)
		enc.SetIndent("", "  ")
		return enc.Encode(f)
	}
	return weather.WriteForecast(cmd.Root().Writer, f)
}

func queryArg(cmd *cli.Command) (string, error) {
	name := cmd.String("name")
	args := strings.Join(cmd.Args().Slice(), " ")
	switch {
	case name != "" && args != "":
		return "", fmt.Errorf("give the municipality either as an argument or with --name, not both")
	case name != "":
		return name, nil
	case args != "":
		return args, nil
	}
	return "", fmt.Errorf("municipality name or id required, e.g. aemet forecast Madrid")
}

// choose resolves the query, asking the user to pick when it is ambiguous.
func choose(cmd *cli.Command, query string) (weather.Place, error) {
	place, err := weather.Resolve(query, cmd.String("province"))
	var amb *weather.AmbiguousError
	if !errors.As(err, &amb) {
		return place, err
	}
	w := cmd.Root().ErrWriter
	if cmd.Bool("non-interactive") {
		fmt.Fprintf(w, "%d municipalities match %q, using %s\n", len(amb.Candidates), query, amb.Candidates[0])
		return amb.Candidates[0], nil
	}
	return pick(cmd.Root().Reader, w, amb)
}

func pick(r io.Reader, w io.Writer, amb *weather.AmbiguousError) (weather.Place, error) {
	fmt.Fprintf(w, "Several municipalities match %q:\n\n", amb.Query)
	for i, c := range amb.Candidates {
		fmt.Fprintf(w, "%2d. %s\n", i+1, c)
	}
	fmt.Fprintf(w, "\nSelect a municipality (1-%d): ", len(amb.Candidates))
	line, err := bufio.NewReader(io.LimitReader(r, 64)).ReadString('\n')
	if err != nil && line == "" {
		return weather.Place{}, fmt.Errorf("no selection made: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(amb.Candidates) {
		return weather.Place{}, fmt.Errorf("invalid selection %q", strings.TrimSpace(line))
	}
	return amb.Candidates[n-1], nil
}
