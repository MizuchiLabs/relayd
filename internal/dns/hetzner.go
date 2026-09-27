package dns

import "github.com/libdns/hetzner/v2"

func newHetzner(env env) client {
	return &hetzner.Provider{APIToken: env("TOKEN")}
}
