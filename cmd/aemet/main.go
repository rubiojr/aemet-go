package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rubiojr/aemet-go/cmd/aemet/day"
	"github.com/rubiojr/aemet-go/cmd/aemet/forecast"
	"github.com/rubiojr/aemet-go/cmd/aemet/mcpserver"
	"github.com/rubiojr/aemet-go/cmd/aemet/radar"
	"github.com/rubiojr/aemet-go/cmd/aemet/search"
	"github.com/rubiojr/aemet-go/cmd/aemet/util"
	"github.com/rubiojr/aemet-go/cmd/aemet/version"
	"github.com/urfave/cli/v3"
)

// newApp builds the root command. Tests use it with custom writers.
func newApp() *cli.Command {
	return &cli.Command{
		Name:    util.AppName,
		Usage:   "Spanish weather forecasts from AEMET",
		Version: util.GetVersion(),
		Flags:   util.GlobalFlags(),
		Commands: []*cli.Command{
			forecast.Command,
			day.Command,
			radar.Command,
			search.Command,
			mcpserver.Command,
			version.Command,
		},
	}
}

func main() {
	if err := newApp().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", util.AppName, err)
		os.Exit(1)
	}
}
