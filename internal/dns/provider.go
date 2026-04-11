// Package dns provides an abstracted interface over various DNS providers using libdns.
package dns

import (
	"context"
	"fmt"

	"github.com/libdns/libdns"
	"github.com/mizuchilabs/relayd/internal/config"
)

type Record struct {
	Type     string
	Name     string
	Value    string
	Original libdns.Record
}

type ChangeSet struct {
	Create []Record
	Update []Record
	Delete []Record
}

type Provider interface {
	Name() string
	Scope() string
	Zones() []string
	Force() bool
	Records(ctx context.Context, zone string) ([]Record, error)
	Apply(ctx context.Context, zone string, changes ChangeSet) error
}

func BuildProviders(cfgs []config.Provider) ([]Provider, error) {
	var providers []Provider
	for _, cfg := range cfgs {
		switch cfg.Type {
		case "cloudflare":
			providers = append(providers, NewCloudflareProvider(cfg))
		case "digitalocean":
			providers = append(providers, NewDigitalOceanProvider(cfg))
		case "route53":
			providers = append(providers, NewRoute53Provider(cfg))
		case "powerdns":
			providers = append(providers, NewPowerDNSProvider(cfg))
		case "unifi":
			providers = append(providers, NewUnifiProvider(cfg))
		case "pihole":
			providers = append(providers, NewPiholeProvider(cfg))
		case "rfc2136":
			providers = append(providers, NewRFC2136Provider(cfg))
		default:
			return nil, fmt.Errorf("unsupported provider type: %s", cfg.Type)
		}
	}
	return providers, nil
}
