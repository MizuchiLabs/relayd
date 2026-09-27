package dns

import (
	"cmp"
	"strings"

	"github.com/libdns/rfc2136"
)

func newRFC2136(env env) client {
	keyName := env("KEY_NAME")
	if keyName != "" {
		keyName = withDot(keyName)
	}
	return &rfc2136.Provider{
		Server:  env("URL"),
		KeyName: keyName,
		KeyAlg:  withDot(cmp.Or(env("KEY_ALGORITHM"), "hmac-sha256")),
		Key:     env("KEY"),
	}
}

func withDot(s string) string {
	return strings.TrimSuffix(s, ".") + "."
}
