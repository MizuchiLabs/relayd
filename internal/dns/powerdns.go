package dns

import (
	"os"
	"strings"

	"github.com/libdns/powerdns"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewPowerDNSProvider creates a new PowerDNS provider wrapped for relayd.
func NewPowerDNSProvider(cfg config.Provider) Provider {
	name := strings.ToUpper(strings.ReplaceAll(cfg.Type, "-", "_"))
	url := os.Getenv("RELAYD_PROVIDER_" + name + "_URL")
	if url == "" {
		url = "http://localhost:8081"
	}

	return &wrapper{
		scope: cfg.Scope,
		zones: append([]string(nil), cfg.Zones...),
		client: &powerdns.Provider{
			ServerURL: url,
			APIToken:  cfg.Token,
		},
	}
}
