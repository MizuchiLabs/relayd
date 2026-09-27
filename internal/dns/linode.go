package dns

import "github.com/libdns/linode"

func newLinode(env env) client {
	return &linode.Provider{APIToken: env("TOKEN")}
}
