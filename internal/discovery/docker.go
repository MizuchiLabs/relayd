// Package discovery finds the hostnames relayd should publish from Docker containers and Swarm services.
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/moby/moby/client"

	"github.com/mizuchilabs/relayd/internal/dns"
)

var (
	hostRule    = regexp.MustCompile(`Host\(([^)]*)\)`)
	quotedValue = regexp.MustCompile("`([^`]*)`|\"([^\"]*)\"|'([^']*)'")
)

// Host is a hostname and the providers (by name or scope) it may be published to. No providers means all of them.
type Host struct {
	Name      string
	Providers []string
	// TTL comes from the relayd.ttl label. Zero means the provider's default.
	TTL time.Duration
}

type Docker struct {
	cli *client.Client
}

// New connects to the Docker daemon from DOCKER_HOST (or the default socket) and closes the connection when ctx is done.
func New(ctx context.Context) (*Docker, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, err
	}
	if _, err := cli.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true}); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("connecting to docker: %w", err)
	}
	context.AfterFunc(ctx, func() { _ = cli.Close() })
	return &Docker{cli: cli}, nil
}

// Hosts lists the hosts of running containers and Swarm services labeled relayd.enable=true.
func (d *Docker) Hosts(ctx context.Context) ([]Host, error) {
	containers, err := d.cli.ContainerList(ctx, client.ContainerListOptions{
		Filters: client.Filters{}.Add("label", "relayd.enable=true"),
	})
	if err != nil {
		return nil, err
	}

	var hosts []Host
	for _, c := range containers.Items {
		hosts = append(hosts, hostsFromLabels(c.Labels)...)
	}

	// relayd.enable can sit on the service or on its container spec, so services can't be filtered by label.
	services, err := d.cli.ServiceList(ctx, client.ServiceListOptions{})
	if err != nil {
		slog.Debug("Skipping swarm services", "error", err)
		return hosts, nil
	}
	for _, svc := range services.Items {
		hosts = append(hosts, hostsFromLabels(svc.Spec.Labels)...)
		if spec := svc.Spec.TaskTemplate.ContainerSpec; spec != nil {
			hosts = append(hosts, hostsFromLabels(spec.Labels)...)
		}
	}
	return hosts, nil
}

func hostsFromLabels(labels map[string]string) []Host {
	if labels["relayd.enable"] != "true" {
		return nil
	}

	var providers []string
	for p := range strings.SplitSeq(labels["relayd.providers"], ",") {
		if p = strings.TrimSpace(p); p != "" {
			providers = append(providers, p)
		}
	}

	ttl, err := dns.ParseTTL(strings.TrimSpace(labels["relayd.ttl"]))
	if err != nil {
		slog.Warn("Ignoring relayd.ttl label", "error", err)
	}

	names := strings.Split(labels["relayd.hosts"], ",")
	for key, value := range labels {
		if !strings.HasPrefix(key, "traefik.http.routers.") || !strings.HasSuffix(key, ".rule") {
			continue
		}
		for _, rule := range hostRule.FindAllStringSubmatch(value, -1) {
			for _, m := range quotedValue.FindAllStringSubmatch(rule[1], -1) {
				names = append(names, m[1]+m[2]+m[3])
			}
		}
	}

	var hosts []Host
	for _, name := range names {
		name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
		if name != "" && !strings.ContainsAny(name, " \t") {
			hosts = append(hosts, Host{Name: name, Providers: providers, TTL: ttl})
		}
	}
	return hosts
}

// Watch signals when containers or services change. Bursts (like compose up) collapse into one signal
// after things go quiet for a second. It also signals after reconnecting, to catch missed events.
func (d *Docker) Watch(ctx context.Context) <-chan struct{} {
	out := make(chan struct{}, 1)
	filters := client.Filters{}.
		Add("type", "container", "service").
		Add("event", "start", "die", "create", "update", "remove")

	go func() {
		debounce := time.NewTimer(time.Hour)
		debounce.Stop()
		events := d.cli.Events(ctx, client.EventsListOptions{Filters: filters})

		for {
			select {
			case <-ctx.Done():
				return
			case <-events.Messages:
				debounce.Reset(time.Second)
			case <-debounce.C:
				select {
				case out <- struct{}{}:
				default:
				}
			case err := <-events.Err:
				slog.Warn("Docker event stream lost, reconnecting", "error", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
				events = d.cli.Events(ctx, client.EventsListOptions{Filters: filters})
				debounce.Reset(0)
			}
		}
	}()
	return out
}
