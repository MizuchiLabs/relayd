package dns

import (
	"os"

	"github.com/libdns/scaleway"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewScalewayProvider creates a new Scaleway DNS provider wrapped for relayd.
func NewScalewayProvider(cfg config.Provider) Provider {
	return newWrapper(cfg, &scaleway.Provider{
		SecretKey:      cfg.Token,
		OrganizationID: os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_ORGANIZATION_ID"),
	})
}
