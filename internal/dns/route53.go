package dns

import "github.com/libdns/route53"

// newRoute53 picks up AWS credentials from the usual AWS env vars and config files.
func newRoute53(env env) client {
	return &route53.Provider{HostedZoneID: env("ZONE_ID")}
}
