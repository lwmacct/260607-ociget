package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
	"github.com/lwmacct/251207-go-pkg-version/pkg/version"
	"github.com/lwmacct/251219-go-pkg-logm/pkg/logm"
	appserver "github.com/lwmacct/260607-ociget/internal/app/server"
	"github.com/lwmacct/260607-ociget/internal/config"
)

func buildCommands() []*cli.Command {
	return []*cli.Command{
		serverCommand(),
		controlCommand(),
		version.Command,
	}
}

func serverCommand() *cli.Command {
	defaults := config.DefaultConfig()
	return &cli.Command{
		Name:            "server",
		Usage:           "启动 Web API 服务",
		Action:          serveAction,
		Commands:        []*cli.Command{version.Command},
		HideHelpCommand: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "http.listen",
				Usage: "HTTP 服务监听地址",
				Value: defaults.Server.HTTP.Listen,
			},
			&cli.StringFlag{
				Name:  "http.ssl-cert-file",
				Usage: "HTTPS TLS 证书文件",
				Value: defaults.Server.HTTP.SSLCertFile,
			},
			&cli.StringFlag{
				Name:  "http.ssl-key-file",
				Usage: "HTTPS TLS 私钥文件",
				Value: defaults.Server.HTTP.SSLKeyFile,
			},
			&cli.StringFlag{
				Name:  "database",
				Usage: "SQLite 数据库文件路径",
				Value: defaults.Server.Database,
			},
			&cli.StringFlag{
				Name:  "control.listen",
				Usage: "Unix socket 控制面监听地址",
				Value: defaults.Server.Control.Listen,
			},
			&cli.BoolFlag{
				Name:  "download-cache.enabled",
				Usage: "启用下载文件缓存",
				Value: defaults.Server.DownloadCache.Enabled,
			},
			&cli.StringFlag{
				Name:  "download-cache.dir",
				Usage: "下载文件缓存目录",
				Value: defaults.Server.DownloadCache.Dir,
			},
			&cli.StringFlag{
				Name:  "download-cache.ttl",
				Usage: "下载文件缓存有效期, 例如 168h",
				Value: defaults.Server.DownloadCache.TTL,
			},
		},
	}
}

func serveAction(ctx context.Context, cmd *cli.Command) error {
	cfg := cfgm.MustLoadCmd(cmd, config.DefaultConfig(), version.AppVersion)
	return appserver.Run(ctx, cfg)
}

func controlCommand() *cli.Command {
	defaults := config.DefaultConfig()
	return &cli.Command{
		Name:  "control",
		Usage: "向本地控制面发送命令",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "socket",
				Usage: "控制面 Unix socket 路径",
				Value: defaults.Server.Control.Listen,
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "reload-cert",
				Usage: "重载 TLS 证书",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					socket := cmd.String("socket")
					if socket == "" {
						return fmt.Errorf("socket is required")
					}
					conn, err := net.Dial("unix", socket)
					if err != nil {
						return err
					}
					defer conn.Close()

					if _, err := io.WriteString(conn, "reload-cert\n"); err != nil {
						return err
					}
					resp, err := io.ReadAll(conn)
					if err != nil {
						return err
					}
					out := strings.TrimSpace(string(resp))
					if strings.HasPrefix(out, "ERR ") {
						return fmt.Errorf("%s", strings.TrimPrefix(out, "ERR "))
					}
					slog.Info("control command completed", "response", out)
					return nil
				},
			},
		},
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

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}
