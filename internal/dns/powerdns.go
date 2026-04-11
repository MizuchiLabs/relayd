package dns

import (
	"github.com/libdns/powerdns"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewPowerDNSProvider creates a new PowerDNS provider wrapped for relayd.
func NewPowerDNSProvider(cfg config.Provider) Provider {
	return newWrapper(cfg, &powerdns.Provider{
		ServerURL: cfg.URL,
		APIToken:  cfg.Token,
	})
}
