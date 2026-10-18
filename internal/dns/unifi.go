package dns

import (
	"cmp"
	"strings"

	"github.com/mizuchilabs/relayd/internal/dns/unifi"
)

func newUnifi(env env) (client, error) {
	// UniFi gateways ship a self-signed cert and are rarely behind a proxy, so verification is opt-in.
	insecure, err := parseBool(env("INSECURE"), true)
	if err != nil {
		return nil, err
	}
	server := env("URL")
	if !strings.HasSuffix(server, "/proxy/network/integration/v1") {
		server = strings.TrimRight(server, "/") + "/proxy/network/integration/v1"
	}
	return &unifi.Provider{
		Server:   server,
		Token:    env("TOKEN"),
		Site:     cmp.Or(env("SITE"), env("SITE_ID")),
		Insecure: insecure,
	}, nil
}
