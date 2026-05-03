// Package engine orchestrates the discovery, target IP resolution, and DNS reconciliation loops.
package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/discovery"
	"github.com/mizuchilabs/relayd/internal/dns"
	"github.com/mizuchilabs/relayd/internal/reconcile"
	"github.com/mizuchilabs/relayd/internal/targets"
)

func Run(ctx context.Context, cfg config.Config) error {
	providers, err := dns.BuildProviders(cfg.Providers)
	if err != nil {
		return err
	}

	slog.Info("Starting relayd", "interval", cfg.Interval)
	source, err := discovery.NewDockerSource()
	if err != nil {
		slog.Error("Docker source failed", "error", err)
		return err
	}
	defer func() { _ = source.Close() }()

	// Initial sync
	update(ctx, cfg, providers, source)

	events := source.Watch(ctx)
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-events:
			update(ctx, cfg, providers, source)
		case <-ticker.C:
			update(ctx, cfg, providers, source)
		}
	}
}

func update(
	ctx context.Context,
	cfg config.Config,
	providers []dns.Provider,
	source *discovery.DockerSource,
) {
	hosts, err := source.ListHostnames(ctx)
	if err != nil {
		slog.Error("Failed to list hostnames", "error", err)
		return
	}

	var resolveGroup sync.WaitGroup

	var localIP, publicIP targets.IPs
	resolveGroup.Go(func() {
		ips, err := targets.ResolveLocalIP(cfg.IPFamily)
		if err == nil {
			localIP = ips
		}
	})

	resolveGroup.Go(func() {
		ips, err := targets.ResolvePublicIP(ctx, cfg.IPFamily)
		if err == nil {
			publicIP = ips
		}
	})
	resolveGroup.Wait()

	var wg sync.WaitGroup
	for _, p := range providers {
		wg.Go(func() {
			var providerHosts []string
			for host, allowed := range hosts {
				isAllowed := false
				for _, a := range allowed {
					if a == "*" || a == p.Name() || a == p.Scope() {
						isAllowed = true
						break
					}
				}
				if isAllowed {
					providerHosts = append(providerHosts, host)
				}
			}

			ips := publicIP
			if p.Scope() == "local" {
				ips = localIP
			}

			switch cfg.IPFamily {
			case "ipv4":
				ips.IPv6 = ""
			case "ipv6":
				ips.IPv4 = ""
			}

			if !ips.HasAny() {
				slog.Warn(
					"No IP available for scope and family",
					"scope",
					p.Scope(),
					"family",
					cfg.IPFamily,
				)
				return
			}

			for _, zone := range p.Zones() {
				if err := reconcile.Apply(ctx, p, cfg.Instance, zone, providerHosts, ips); err != nil {
					slog.Error("Sync failed", "provider", p.Name(), "zone", zone, "error", err)
				}
			}
		})
	}
	wg.Wait()
}
