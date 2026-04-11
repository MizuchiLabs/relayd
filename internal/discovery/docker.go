// Package discovery is responsible for locating services and extracting their desired hostnames.
package discovery

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
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
	containers, err := s.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, err
	}

	hosts := make(map[string]map[string]bool)
	for _, c := range containers {
		if c.Labels["relayd.enable"] != "true" {
			continue // Skip non-relayd containers
		}

		providerMap := make(map[string]bool)
		if pVal, ok := c.Labels["relayd.providers"]; ok && pVal != "" {
			for p := range strings.SplitSeq(pVal, ",") {
				providerMap[strings.TrimSpace(p)] = true
			}
		}

		for _, host := range extractHostnames(c.Labels) {
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

func (s *DockerSource) Watch(ctx context.Context) (<-chan Event, <-chan error) {
	args := filters.NewArgs()
	args.Add("type", "container")

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
				}
				return
			case msg, ok := <-msgs:
				if !ok {
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
	case "start", "restart", "die", "stop", "destroy", "rename", "update":
		return true
	}
	return false
}
