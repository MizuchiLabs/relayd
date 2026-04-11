package dns

import (
	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/dns/pihole"
)

// NewPiholeProvider creates a new Pi-hole provider wrapped for relayd.
func NewPiholeProvider(cfg config.Provider) Provider {
	return newWrapper(cfg, &pihole.Provider{
		Server:   cfg.URL,
		Password: cfg.Token,
	})
}
