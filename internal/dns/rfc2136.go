package dns

import (
	"os"
	"strings"

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
	if !strings.HasSuffix(keyAlgorithm, ".") {
		keyAlgorithm += "."
	}
	if keyName != "" && !strings.HasSuffix(keyName, ".") {
		keyName += "."
	}

	return newWrapper(cfg, &rfc2136.Provider{
		Server:  cfg.URL,
		KeyName: keyName,
		KeyAlg:  keyAlgorithm,
		Key:     key,
	})
}
