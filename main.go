package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mizuchilabs/kata/buildinfo"
	"github.com/mizuchilabs/kata/logx"
	"github.com/mizuchilabs/kata/sigx"
	"github.com/urfave/cli/v3"

	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/engine"
)

func main() {
	cmd := &cli.Command{
		EnableShellCompletion: true,
		Suggest:               true,
		Name:                  "relayd",
		Version:               buildinfo.String(),
		Usage:                 "keeps your DNS records in sync",
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			logx.Init(cmd.Bool("debug"))

			if _, err := os.Stat("/var/run/docker.sock"); err != nil {
				slog.Warn("Docker socket not found", "path", "/var/run/docker.sock")
			}
			return ctx, nil
		},
		DefaultCommand: "start",
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start the relayd synchronization engine",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return engine.Run(ctx, config.New(cmd))
				},
			},
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
				Usage:   "Enable debug logging",
				Sources: cli.EnvVars("RELAYD_DEBUG"),
			},
			&cli.DurationFlag{
				Name:    "interval",
				Usage:   "Time interval for recurring background DNS synchronization (e.g. 5m, 1h)",
				Value:   5 * time.Minute,
				Sources: cli.EnvVars("RELAYD_INTERVAL"),
			},
			&cli.StringFlag{
				Name:    "instance",
				Usage:   "Unique identifier for the relayd instance (e.g. 'my-instance')",
				Sources: cli.EnvVars("RELAYD_INSTANCE"),
			},
			&cli.StringFlag{
				Name:    "ip-family",
				Aliases: []string{"f"},
				Usage:   "IP family to synchronize: ipv4, ipv6, or dual",
				Value:   "ipv4",
				Sources: cli.EnvVars("RELAYD_IP_FAMILY"),
			},
		},
	}

	if err := cmd.Run(sigx.NotifyContext(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cmd.Name, err)
		os.Exit(1)
	}
}
