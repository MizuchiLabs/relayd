package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/engine"
	"github.com/urfave/cli/v3"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func main() {
	cmd := &cli.Command{
		EnableShellCompletion: true,
		Suggest:               true,
		Name:                  "relayd",
		Version:               fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, Date),
		Usage:                 "keeps your DNS records in sync",
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			level := slog.LevelInfo
			if cmd.Bool("debug") {
				level = slog.LevelDebug
			}
			slog.SetDefault(
				slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})),
			)

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

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := cmd.Run(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}
