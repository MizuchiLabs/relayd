package dns

import (
	"os"
	"strings"

	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/dns/unifi"
)

// NewUnifiProvider creates a new UniFi DNS provider wrapped for relayd.
func NewUnifiProvider(cfg config.Provider) Provider {
	baseURL := cfg.URL
	if !strings.HasSuffix(baseURL, "/proxy/network/integration/v1") {
		baseURL = strings.TrimRight(baseURL, "/") + "/proxy/network/integration/v1"
	}

	return newWrapper(cfg, &unifi.Provider{
		Server: baseURL,
		Token:  cfg.Token,
		Site:   os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_SITE"),
	})
}
