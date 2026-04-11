package dns

import (
	"os"

	"github.com/libdns/unifi"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewUnifiProvider creates a new UniFi DNS provider wrapped for relayd.
func NewUnifiProvider(cfg config.Provider) Provider {
	siteID := os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_SITE_ID")
	if siteID == "" {
		siteID = "default"
	}

	return &wrapper{
		scope: cfg.Scope,
		zones: append([]string(nil), cfg.Zones...),
		client: &unifi.Provider{
			SiteId:  siteID,
			ApiKey:  cfg.Token,
			BaseUrl: cfg.URL,
		},
	}
}
