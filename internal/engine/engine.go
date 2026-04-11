// Package engine orchestrates the discovery, target IP resolution, and DNS reconciliation loops.
package engine

import (
	"context"
	"log/slog"
	"strings"
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

	slog.Info("Starting relayd", "interval", cfg.Interval)
	source, err := discovery.NewDockerSource()
	if err != nil {
		slog.Error("Docker source failed", "error", err)
		return err
	}
	defer func() { _ = source.Close() }()

	if err := syncAll(ctx, providers, source); err != nil {
		slog.Error("Initial sync failed", "error", err)
	}

	events, watchErrs := source.Watch(ctx)
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	var debounceTimer *time.Timer
	var debounce <-chan time.Time
	debounceDuration := 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-watchErrs:
			slog.Error("Docker watcher encountered an error", "error", err)
			return err
		case ev := <-events:
			slog.Debug("Docker event received", "action", ev.Action, "id", ev.ID)
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(debounceDuration)
				debounce = debounceTimer.C
			} else {
				debounceTimer.Reset(debounceDuration)
			}
		case <-debounce:
			debounceTimer.Stop()
			debounceTimer = nil
			debounce = nil
			slog.Debug("Debounced Docker events, triggering sync")
			if err := syncAll(ctx, providers, source); err != nil {
				slog.Error("Event-triggered sync failed", "error", err)
			}
		case <-ticker.C:
			slog.Debug("Background sync triggered")
			if err := syncAll(ctx, providers, source); err != nil {
				slog.Error("Background sync failed", "error", err)
			}
		}
	}
}

func syncAll(
	ctx context.Context,
	providers []dns.Provider,
	source *discovery.DockerSource,
) error {
	hosts, err := source.ListHostnames(ctx)
	if err != nil {
		return err
	}

	resolveGroup, resolveCtx := errgroup.WithContext(ctx)

	var localIP, publicIP targets.IPs

	resolveGroup.Go(func() error {
		ips, err := targets.ResolveLocalIP()
		if err == nil {
			localIP = ips
		}
		return nil
	})

	resolveGroup.Go(func() error {
		ips, err := targets.ResolvePublicIP(resolveCtx)
		if err == nil {
			publicIP = ips
		}
		return nil
	})

	_ = resolveGroup.Wait()

	slog.Info(
		"Syncing",
		"hosts",
		len(hosts),
		"local_v4",
		strings.Join(localIP.IPv4, ","),
		"local_v6",
		strings.Join(localIP.IPv6, ","),
		"public_v4",
		strings.Join(publicIP.IPv4, ","),
		"public_v6",
		strings.Join(publicIP.IPv6, ","),
	)

	g, gCtx := errgroup.WithContext(ctx)

	for _, p := range providers {
		g.Go(func() error {
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
			if !ips.HasAny() {
				slog.Warn("No IP available for scope", "scope", p.Scope())
				return nil
			}

			for _, zone := range p.Zones() {
				if err := reconcile.Apply(gCtx, p, zone, providerHosts, ips); err != nil {
					slog.Error("Sync failed", "provider", p.Name(), "zone", zone, "error", err)
				}
			}
			return nil
		})
	}

	return g.Wait()
}
