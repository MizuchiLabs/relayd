package dns

import "github.com/libdns/scaleway"

func newScaleway(env env) client {
	return &scaleway.Provider{SecretKey: env("TOKEN"), OrganizationID: env("ORGANIZATION_ID")}
}
