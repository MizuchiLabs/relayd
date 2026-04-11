package dns

import (
	"os"

	"github.com/libdns/rfc2136"
	"github.com/mizuchilabs/relayd/internal/config"
)

// NewRFC2136Provider creates a new RFC2136 provider wrapped for relayd.
func NewRFC2136Provider(cfg config.Provider) Provider {
	keyName := os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_KEY_NAME")
	keyAlgorithm := os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_KEY_ALGORITHM")
	key := os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_KEY")

	if keyAlgorithm == "" {
		keyAlgorithm = "hmac-sha256."
	}

	return &wrapper{
		scope: cfg.Scope,
		zones: append([]string(nil), cfg.Zones...),
		client: &rfc2136.Provider{
			Server:  cfg.URL,
			KeyName: keyName,
			KeyAlg:  keyAlgorithm,
			Key:     key,
		},
	}
}
