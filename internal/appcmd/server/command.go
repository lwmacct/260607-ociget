package server

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/lwmacct/251207-go-pkg-version/pkg/version"
	"github.com/lwmacct/260607-ociget/internal/config"
)

// Command starts the HTTP server.
var Command = &cli.Command{
	Name:            "server",
	Usage:           "启动 Web API 服务",
	Action:          config.Manager.Action(action),
	Commands:        []*cli.Command{version.Command},
	HideHelpCommand: true,
}

func action(ctx context.Context, _ *cli.Command, cfg *config.Config) error {
	return NewApp(&cfg.Server).Run(ctx)
}
