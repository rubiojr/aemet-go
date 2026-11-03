// Package version implements the version command.
package version

import (
	"context"
	"fmt"

	"github.com/rubiojr/aemet-go/cmd/aemet/util"
	"github.com/urfave/cli/v3"
)

// Command prints the CLI version.
var Command = &cli.Command{
	Name:  "version",
	Usage: "Print the version",
	Action: func(ctx context.Context, cmd *cli.Command) error {
		_, err := fmt.Fprintf(cmd.Root().Writer, "%s version %s\n", util.AppName, util.GetVersion())
		return err
	},
}
