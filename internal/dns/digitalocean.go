package dns

import (
	"github.com/libdns/digitalocean"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewDigitalOceanProvider creates a new DigitalOcean DNS provider wrapped for relayd.
func NewDigitalOceanProvider(cfg config.Provider) Provider {
	return &wrapper{
		scope: cfg.Scope,
		zones: append([]string(nil), cfg.Zones...),
		client: &digitalocean.Provider{
			APIToken: cfg.Token,
		},
	}
}
