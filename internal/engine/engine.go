// Package engine orchestrates the discovery, target IP resolution, and DNS reconciliation loops.
package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/discovery"
	"github.com/mizuchilabs/relayd/internal/dns"
	"github.com/mizuchilabs/relayd/internal/reconcile"
	"github.com/mizuchilabs/relayd/internal/targets"
	"golang.org/x/sync/errgroup"
)

func Run(ctx context.Context, cfg config.Config) error {
	providers, err := dns.BuildProviders(cfg.Providers)
	if err != nil {
		return err
	}

	slog.Info("Starting relayd", "interval", cfg.SyncInterval)
	source, err := discovery.NewDockerSource()
	if err != nil {
		slog.Error("Docker source failed", "error", err)
		return err
	}
	defer func() { _ = source.Close() }()

	if err := syncAll(ctx, cfg, providers, source); err != nil {
		slog.Error("Initial sync failed", "error", err)
	}

	events, watchErrs := source.Watch(ctx)
	ticker := time.NewTicker(cfg.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-watchErrs:
			slog.Error("Docker watcher encountered an error", "error", err)
			return err
		case ev := <-events:
			slog.Debug("Docker event triggered sync", "action", ev.Action, "id", ev.ID)
			if err := syncAll(ctx, cfg, providers, source); err != nil {
				slog.Error("Event-triggered sync failed", "error", err)
			}
		case <-ticker.C:
			slog.Debug("Background sync triggered")
			if err := syncAll(ctx, cfg, providers, source); err != nil {
				slog.Error("Background sync failed", "error", err)
			}
		}
	}
}

func syncAll(
	ctx context.Context,
	cfg config.Config,
	providers []dns.Provider,
	source *discovery.DockerSource,
) error {
	hosts, err := source.ListHostnames(ctx)
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return nil
	}

	g, gCtx := errgroup.WithContext(ctx)

	var localIP, publicIP targets.IPs

	g.Go(func() error {
		ips, err := targets.ResolveLocalIP()
		if err == nil {
			localIP = ips
		}
		return nil
	})

	g.Go(func() error {
		ips, err := targets.ResolvePublicIP(gCtx)
		if err == nil {
			publicIP = ips
		}
		return nil
	})

	_ = g.Wait()

	slog.Info(
		"Syncing",
		"hosts",
		len(hosts),
		"local_v4",
		localIP.IPv4,
		"local_v6",
		localIP.IPv6,
		"public_v4",
		publicIP.IPv4,
		"public_v6",
		publicIP.IPv6,
	)

	for _, p := range providers {
		g.Go(func() error {
			ips := publicIP
			if p.Scope() == "local" {
				ips = localIP
			}
			if !ips.HasAny() {
				slog.Warn("No IP available for scope", "scope", p.Scope())
				return nil
			}

			for _, zone := range p.Zones() {
				if err := reconcile.Apply(gCtx, p, zone, cfg, hosts, ips); err != nil {
					slog.Error("Sync failed", "provider", p.Scope(), "zone", zone, "error", err)
				}
			}
			return nil
		})
	}

	return g.Wait()
}
