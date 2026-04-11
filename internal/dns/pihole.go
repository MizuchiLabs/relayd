package dns

import (
	"os"

	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/dns/pihole"
)

// NewPiholeProvider creates a new Pi-hole provider wrapped for relayd.
func NewPiholeProvider(cfg config.Provider) Provider {
	url := os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_URL")
	if url == "" {
		url = "http://pi.hole"
	}

	return &wrapper{
		scope: cfg.Scope,
		zones: append([]string(nil), cfg.Zones...),
		client: &pihole.Provider{
			Server:   url,
			Password: cfg.Token,
		},
	}
}
