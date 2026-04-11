package dns

import (
	"os"

	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/dns/unifi"
)

// NewUnifiProvider creates a new UniFi DNS provider wrapped for relayd.
func NewUnifiProvider(cfg config.Provider) Provider {
	siteID := os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_SITE_ID")
	if siteID == "" {
		siteID = "default"
	}

	return newWrapper(cfg, &unifi.Provider{
		Server: cfg.URL,
		Token:  cfg.Token,
		SiteID: siteID,
	})
}
