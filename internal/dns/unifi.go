package dns

import (
	"os"
	"strings"

	"github.com/libdns/unifi"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewUnifiProvider creates a new UniFi DNS provider wrapped for relayd.
func NewUnifiProvider(cfg config.Provider) Provider {
	name := strings.ToUpper(strings.ReplaceAll(cfg.Type, "-", "_"))
	apiKey := os.Getenv("RELAYD_PROVIDER_" + name + "_API_KEY")
	siteID := os.Getenv("RELAYD_PROVIDER_" + name + "_SITE_ID")
	baseURL := os.Getenv("RELAYD_PROVIDER_" + name + "_BASE_URL")

	if siteID == "" {
		siteID = "default"
	}

	return &wrapper{
		scope: cfg.Scope,
		zones: append([]string(nil), cfg.Zones...),
		client: &unifi.Provider{
			ApiKey:  apiKey,
			SiteId:  siteID,
			BaseUrl: baseURL,
		},
	}
}
