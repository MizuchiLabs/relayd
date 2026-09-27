// Package engine runs the sync loop: discover hosts, resolve IPs, reconcile every provider.
package engine

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mizuchilabs/relayd/internal/discovery"
	"github.com/mizuchilabs/relayd/internal/dns"
	"github.com/mizuchilabs/relayd/internal/reconcile"
	"github.com/mizuchilabs/relayd/internal/targets"
)

type Options struct {
	Instance string
	Interval time.Duration
	IPFamily string
	DryRun   bool
}

type engine struct {
	Options

	providers []*dns.Provider
	docker    *discovery.Docker
	resolver  *targets.Resolver
}

// Run syncs once, then again on every Docker change and every interval, until ctx is done.
func Run(ctx context.Context, opts Options) error {
	opts.Instance = cmp.Or(opts.Instance, hostname())

	providers, err := dns.LoadProviders()
	if err != nil {
		return err
	}
	if len(providers) == 0 {
		return errors.New("no providers configured, set RELAYD_PROVIDER_<NAME>_TYPE")
	}
	resolver, err := targets.NewResolver(opts.IPFamily)
	if err != nil {
		return err
	}
	docker, err := discovery.New(ctx)
	if err != nil {
		return err
	}

	e := &engine{Options: opts, providers: providers, docker: docker, resolver: resolver}
	slog.Info("Starting relayd",
		"instance", opts.Instance, "interval", opts.Interval, "providers", len(providers), "dry_run", opts.DryRun)

	// Container events only change hostnames, so IPs are refreshed on the interval
	// (or when the last attempt came up empty).
	ips := e.resolveIPs(ctx)
	e.sync(ctx, ips)

	events := docker.Watch(ctx)
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			ips = e.resolveIPs(ctx)
		case <-events:
			if slices.ContainsFunc(e.providers, func(p *dns.Provider) bool { return !ips[p.Scope].HasAny() }) {
				ips = e.resolveIPs(ctx)
			}
		}
		e.sync(ctx, ips)
	}
}

func hostname() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "default"
}

// resolveIPs resolves each scope that a provider uses, keyed by scope.
func (e *engine) resolveIPs(ctx context.Context) map[string]targets.IPs {
	out := map[string]targets.IPs{}
	for _, p := range e.providers {
		if _, done := out[p.Scope]; done {
			continue
		}
		ips, err := e.resolver.Resolve(ctx, p.Scope)
		if err != nil {
			slog.Warn("Could not resolve IP", "scope", p.Scope, "error", err)
		}
		out[p.Scope] = ips
	}
	return out
}

func (e *engine) sync(ctx context.Context, ips map[string]targets.IPs) {
	hosts, err := e.docker.Hosts(ctx)
	if err != nil {
		slog.Error("Failed to list hosts", "error", err)
		return
	}

	var wg sync.WaitGroup
	for _, p := range e.providers {
		wg.Go(func() { e.syncProvider(ctx, p, hostsFor(p, hosts), ips[p.Scope]) })
	}
	wg.Wait()
}

func (e *engine) syncProvider(ctx context.Context, p *dns.Provider, hosts map[string]time.Duration, ips targets.IPs) {
	if !ips.HasAny() {
		slog.Warn("No IP to publish, skipping provider", "provider", p.Name, "scope", p.Scope)
		return
	}

	for _, zone := range p.Zones {
		log := slog.With("provider", p.Name, "zone", zone)
		records, err := p.Records(ctx, zone)
		if err != nil {
			log.Error("Failed to fetch records", "error", err)
			continue
		}

		changes := reconcile.Plan(reconcile.Desired{
			Zone:     zone,
			Instance: e.Instance,
			Hosts:    hosts,
			IPs:      ips,
			Force:    p.Force,
		}, records)
		if changes.Empty() {
			continue
		}

		level := slog.LevelDebug
		if e.DryRun {
			level = slog.LevelInfo
		}
		for _, r := range changes.Create {
			log.Log(ctx, level, "Create record", "type", r.Type, "name", r.Name, "value", r.Value, "ttl", r.TTL)
		}
		for _, r := range changes.Update {
			log.Log(ctx, level, "Update record TTL", "type", r.Type, "name", r.Name, "ttl", r.TTL)
		}
		for _, r := range changes.Delete {
			log.Log(ctx, level, "Delete record", "type", r.Type, "name", r.Name, "value", r.Value)
		}
		if e.DryRun {
			continue
		}

		log.Info("Applying changes",
			"create", len(changes.Create), "update", len(changes.Update), "delete", len(changes.Delete))
		if err := p.Apply(ctx, zone, changes); err != nil {
			log.Error("Failed to apply changes", "error", err)
		}
	}
}

// hostsFor returns the hosts a provider should publish (matched on the relayd.providers label) with their TTL.
func hostsFor(p *dns.Provider, hosts []discovery.Host) map[string]time.Duration {
	out := map[string]time.Duration{}
	for _, h := range hosts {
		if len(h.Providers) > 0 && !slices.ContainsFunc(h.Providers, func(want string) bool {
			return want == "*" || strings.EqualFold(want, p.Name) || strings.EqualFold(want, p.Scope)
		}) {
			continue
		}
		ttl := cmp.Or(h.TTL, p.TTL)
		if p.IgnoreTTL {
			ttl = 0
		}
		// When containers disagree on a host's TTL, the lowest explicit one wins.
		if cur, ok := out[h.Name]; ok && cur != 0 && (ttl == 0 || cur < ttl) {
			continue
		}
		out[h.Name] = ttl
	}
	return out
}
