package dns

import (
	"cmp"
	"strings"

	"github.com/mizuchilabs/relayd/internal/dns/unifi"
)

func newUnifi(env env) (client, error) {
	insecure, err := parseBool(env("INSECURE"), false)
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
