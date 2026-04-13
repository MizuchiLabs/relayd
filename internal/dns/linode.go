package dns

import (
	"github.com/libdns/linode"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewLinodeProvider creates a new Linode DNS provider wrapped for relayd.
func NewLinodeProvider(cfg config.Provider) Provider {
	return newWrapper(cfg, &linode.Provider{
		APIToken: cfg.Token,
	})
}
