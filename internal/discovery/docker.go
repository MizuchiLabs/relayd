// Package discovery is responsible for locating services and extracting their desired hostnames.
package discovery

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/mizuchilabs/relayd/internal/util"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

type Event struct {
	Action string
	ID     string
}

type DockerSource struct {
	cli *client.Client
}

var hostRuleRegex = regexp.MustCompile(`Host\(([^)]*)\)`)

func NewDockerSource() (*DockerSource, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &DockerSource{cli: cli}, nil
}

func (s *DockerSource) Close() error {
	return s.cli.Close()
}

func (s *DockerSource) ListHostnames(ctx context.Context) (map[string][]string, error) {
	hosts := make(map[string]map[string]bool)

	filters := client.Filters{}
	filters.Add("label", "relayd.enable=true")
	containers, err := s.cli.ContainerList(ctx, client.ContainerListOptions{Filters: filters})
	if err != nil {
		return nil, err
	}

	for _, c := range containers.Items {
		processLabels(c.Labels, hosts)
	}

	// Fetch swarm services (ignoring errors if not a swarm manager)
	services, err := s.cli.ServiceList(ctx, client.ServiceListOptions{})
	if err == nil {
		for _, svc := range services.Items {
			// Check both service-level and container-level labels
			processLabels(svc.Spec.Labels, hosts)
			if svc.Spec.TaskTemplate.ContainerSpec != nil {
				processLabels(svc.Spec.TaskTemplate.ContainerSpec.Labels, hosts)
			}
		}
	}

	out := make(map[string][]string)
	for host, pm := range hosts {
		if pm["*"] {
			out[host] = []string{"*"}
		} else {
			var plist []string
			for p := range pm {
				plist = append(plist, p)
			}
			out[host] = plist
		}
	}
	return out, nil
}

func processLabels(labels map[string]string, hosts map[string]map[string]bool) {
	if labels == nil || labels["relayd.enable"] != "true" {
		return // Skip non-relayd containers
	}

	providerMap := make(map[string]bool)
	if pVal, ok := labels["relayd.providers"]; ok && pVal != "" {
		for p := range strings.SplitSeq(pVal, ",") {
			providerMap[strings.TrimSpace(p)] = true
		}
	}

	for _, host := range extractHostnames(labels) {
		if hosts[host] == nil {
			hosts[host] = make(map[string]bool)
		}
		if len(providerMap) == 0 {
			hosts[host]["*"] = true
		} else {
			for p := range providerMap {
				hosts[host][p] = true
			}
		}
	}
}

func extractHostnames(labels map[string]string) []string {
	var hosts []string

	// Manual label
	if val, ok := labels["relayd.hosts"]; ok {
		for v := range strings.SplitSeq(val, ",") {
			if h := util.NormalizeHostname(v); h != "" {
				hosts = append(hosts, h)
			}
		}
	}

	// Traefik Extract
	for key, value := range labels {
		if !strings.HasPrefix(key, "traefik.http.routers.") || !strings.HasSuffix(key, ".rule") {
			continue
		}
		for _, match := range hostRuleRegex.FindAllStringSubmatch(value, -1) {
			if len(match) > 1 {
				hosts = append(hosts, util.ParseQuotedValues(match[1])...)
			}
		}
	}

	return hosts
}

func (s *DockerSource) Watch(ctx context.Context) <-chan Event {
	var stream <-chan events.Message
	var errs <-chan error

	out := make(chan Event, 100)
	startStream := func() {
		filters := client.Filters{}

		// Standalone Containers
		filters.Add("type", "container")
		filters.Add("event", "start")
		filters.Add("event", "die")

		// Swarm Services
		filters.Add("type", "service")
		filters.Add("event", "create")
		filters.Add("event", "update")
		filters.Add("event", "remove")

		res := s.cli.Events(ctx, client.EventsListOptions{Filters: filters})
		stream = res.Messages
		errs = res.Err
	}

	startStream()

	var debounceTimer *time.Timer
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errs:
				if !ok || err != nil {
					if ctx.Err() != nil {
						return
					}
					if err != nil {
						slog.Error("Docker event error", "error", err)
					}
					time.Sleep(3 * time.Second)
					startStream()
					out <- Event{Action: "reconnect"}
				}

			case msg, ok := <-stream:
				if !ok {
					if ctx.Err() != nil {
						return
					}
					slog.Warn("Docker event stream closed, reconnecting...")
					time.Sleep(3 * time.Second)
					startStream()
					out <- Event{Action: "reconnect"}
					continue
				}

				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(100*time.Millisecond, func() {
					out <- Event{Action: string(msg.Action), ID: msg.Actor.ID}
				})
			}
		}
	}()

	return out
}
