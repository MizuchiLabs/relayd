// Package discovery is responsible for locating services and extracting their desired hostnames.
package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

type Event struct {
	Action string
	ID     string
}

type DockerSource struct {
	client *client.Client
}

func NewDockerSource() (*DockerSource, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &DockerSource{client: cli}, nil
}

func (s *DockerSource) Close() error {
	return s.client.Close()
}

func (s *DockerSource) ListHostnames(ctx context.Context) (map[string][]string, error) {
	hosts := make(map[string]map[string]bool)

	containers, err := s.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		processLabels(c.Labels, hosts)
	}

	// Fetch swarm services (ignoring errors if not a swarm manager)
	services, err := s.client.ServiceList(ctx, swarm.ServiceListOptions{})
	if err == nil {
		for _, svc := range services {
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

func (s *DockerSource) Watch(ctx context.Context) (<-chan Event, <-chan error) {
	args := filters.NewArgs()
	args.Add("type", "container")
	args.Add("type", "service")

	msgs, errs := s.client.Events(ctx, events.ListOptions{Filters: args})

	out := make(chan Event)
	errOut := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errOut)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errs:
				if ok {
					errOut <- err
				} else {
					errOut <- fmt.Errorf("docker event stream closed unexpectedly")
				}
				return
			case msg, ok := <-msgs:
				if !ok {
					errOut <- fmt.Errorf("docker event stream closed unexpectedly")
					return
				}
				if isRelevantAction(string(msg.Action)) {
					out <- Event{Action: string(msg.Action), ID: msg.Actor.ID}
				}
			}
		}
	}()

	return out, errOut
}

func isRelevantAction(action string) bool {
	switch action {
	case "start", "restart", "die", "stop", "destroy", "rename", "update", "create", "remove":
		return true
	}
	return false
}
