package dns

import (
	libdnscloudflare "github.com/libdns/cloudflare"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewCloudflareProvider creates a new Cloudflare DNS provider wrapped for relayd.
func NewCloudflareProvider(cfg config.Provider) Provider {
	return &wrapper{
		scope: cfg.Scope,
		zones: append([]string(nil), cfg.Zones...),
		client: &libdnscloudflare.Provider{
			APIToken: cfg.Token,
		},
	}
}
