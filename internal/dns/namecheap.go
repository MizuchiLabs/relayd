package dns

import "github.com/libdns/namecheap"

func newNamecheap(env env) client {
	return &namecheap.Provider{APIKey: env("TOKEN"), User: env("USER")}
}
