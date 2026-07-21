package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/lwmacct/251207-go-pkg-version/pkg/version"
	"github.com/lwmacct/251219-go-pkg-logm/pkg/logm"
	appserver "github.com/lwmacct/260607-ociget/internal/appcmd/server"
	"github.com/lwmacct/260607-ociget/internal/config"
)

func buildCommands() []*cli.Command {
	return []*cli.Command{
		appserver.Command,
		version.Command,
	}
}

func main() {
	logm.MustInit(logm.PresetAuto())

	cmd := &cli.Command{
		Name:            "webapp",
		Usage:           "Web app skeleton",
		Version:         version.AppVersion,
		Commands:        buildCommands(),
		HideHelpCommand: true,
		Action: func(ctx context.Context, c *cli.Command) error {
			return cli.ShowSubcommandHelp(c)
		},
	}
	config.Manager.MustConfigure(cmd)

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}
