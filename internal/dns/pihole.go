package dns

import (
	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/dns/pihole"
)

// NewPiholeProvider creates a new Pi-hole provider wrapped for relayd.
// Pi-hole does not support TXT records, so force mode is always enabled
// to ensure proper cleanup of stale records without TXT-based ownership tracking.
func NewPiholeProvider(cfg config.Provider) Provider {
	cfg.Force = true
	return newWrapper(cfg, &pihole.Provider{
		Server:   cfg.URL,
		Password: cfg.Token,
	})
}
