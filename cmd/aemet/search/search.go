// Package search implements the search command.
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rubiojr/aemet-go/cmd/aemet/weather"
	"github.com/urfave/cli/v3"
)

// Command lists municipalities matching a name. It needs no API key.
var Command = &cli.Command{
	Name:      "search",
	Aliases:   []string{"s"},
	Usage:     "Find municipalities and their INE ids (no API key needed)",
	ArgsUsage: "<name>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "province",
			Aliases: []string{"p"},
			Usage:   "only match municipalities in this province",
		},
		&cli.IntFlag{
			Name:  "limit",
			Value: 20,
			Usage: "maximum results (0 for all)",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "print results as JSON",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		query := strings.Join(cmd.Args().Slice(), " ")
		if query == "" {
			return fmt.Errorf("a name to search for is required, e.g. aemet search coruña")
		}
		places, err := weather.Search(query, cmd.String("province"), cmd.Int("limit"))
		if err != nil {
			return err
		}
		out := cmd.Root().Writer
		if cmd.Bool("json") {
			if places == nil {
				places = []weather.Place{}
			}
			return json.NewEncoder(out).Encode(places)
		}
		if len(places) == 0 {
			return fmt.Errorf("no municipality matches %q", query)
		}
		for _, p := range places {
			fmt.Fprintf(out, "%s\t%s\t%s\n", p.ID, p.Name, p.Province)
		}
		return nil
	},
}
