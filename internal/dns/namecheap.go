package dns

import (
	"os"

	"github.com/libdns/namecheap"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewNamecheapProvider creates a new Namecheap DNS provider wrapped for relayd.
func NewNamecheapProvider(cfg config.Provider) Provider {
	return newWrapper(cfg, &namecheap.Provider{
		APIKey: cfg.Token,
		User:   os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_USER"),
	})
}
