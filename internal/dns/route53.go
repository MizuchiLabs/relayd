package dns

import (
	"os"

	"github.com/libdns/route53"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewRoute53Provider creates a new Route53 DNS provider wrapped for relayd.
func NewRoute53Provider(cfg config.Provider) Provider {
	return newWrapper(cfg, &route53.Provider{
		HostedZoneID: os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_ZONE_ID"),
	})
}
