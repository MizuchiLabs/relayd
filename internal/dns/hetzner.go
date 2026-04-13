package dns

import (
	"github.com/libdns/hetzner"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewHetznerProvider creates a new Hetzner DNS provider wrapped for relayd.
func NewHetznerProvider(cfg config.Provider) Provider {
	return newWrapper(cfg, &hetzner.Provider{
		AuthAPIToken: cfg.Token,
	})
}
