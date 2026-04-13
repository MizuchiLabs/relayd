package dns

import (
	"github.com/libdns/hetzner/v2"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewHetznerProvider creates a new Hetzner DNS provider wrapped for relayd.
func NewHetznerProvider(cfg config.Provider) Provider {
	return newWrapper(cfg, &hetzner.Provider{
		APIToken: cfg.Token,
	})
}
