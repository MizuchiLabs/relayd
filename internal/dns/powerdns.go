package dns

import "github.com/libdns/powerdns"

func newPowerDNS(env env) client {
	return &powerdns.Provider{ServerURL: env("URL"), APIToken: env("TOKEN")}
}
