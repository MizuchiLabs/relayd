package dns

import "github.com/libdns/digitalocean"

func newDigitalOcean(env env) client {
	return &digitalocean.Provider{APIToken: env("TOKEN")}
}
