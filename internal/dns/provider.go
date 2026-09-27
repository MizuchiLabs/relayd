// Package dns loads the configured DNS providers and talks to them through libdns.
package dns

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

type client interface {
	libdns.RecordGetter
	libdns.RecordAppender
	libdns.RecordDeleter
}

// defaultTTL is for providers where a zero TTL really means zero instead of auto.
const defaultTTL = 5 * time.Minute

// env reads RELAYD_PROVIDER_<NAME>_<key> for one provider.
type env func(key string) string

type Provider struct {
	Name  string
	Scope string
	Zones []string
	Force bool
	// TTL is the default for this provider's records. Zero leaves it to the provider (auto or zone default).
	TTL time.Duration
	// IgnoreTTL is set for providers that have no TTL at all.
	IgnoreTTL bool
	client    client
}

// LoadProviders builds a provider for every RELAYD_PROVIDER_<NAME>_TYPE variable.
func LoadProviders() ([]*Provider, error) {
	var providers []*Provider
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		name, ok := strings.CutPrefix(key, "RELAYD_PROVIDER_")
		if !ok {
			continue
		}
		if name, ok = strings.CutSuffix(name, "_TYPE"); !ok || name == "" {
			continue
		}
		p, err := newProvider(name, func(key string) string {
			return strings.TrimSpace(os.Getenv("RELAYD_PROVIDER_" + name + "_" + key))
		})
		if err != nil {
			return nil, fmt.Errorf("provider %s: %w", name, err)
		}
		providers = append(providers, p)
	}
	slices.SortFunc(providers, func(a, b *Provider) int { return cmp.Compare(a.Name, b.Name) })
	return providers, nil
}

func newProvider(name string, env env) (*Provider, error) {
	p := &Provider{
		Name:  name,
		Scope: strings.ToLower(cmp.Or(env("SCOPE"), "public")),
	}
	if p.Scope != "public" && p.Scope != "local" {
		return nil, fmt.Errorf("scope must be public or local, got %q", p.Scope)
	}

	for zone := range strings.SplitSeq(env("ZONES"), ",") {
		if zone = strings.ToLower(strings.TrimSpace(zone)); zone != "" {
			p.Zones = append(p.Zones, zone)
		}
	}
	if len(p.Zones) == 0 {
		return nil, errors.New("ZONES is required")
	}

	var err error
	if p.Force, err = parseBool(env("FORCE"), false); err != nil {
		return nil, fmt.Errorf("FORCE: %w", err)
	}
	if p.TTL, err = ParseTTL(env("TTL")); err != nil {
		return nil, fmt.Errorf("TTL: %w", err)
	}

	switch typ := env("TYPE"); typ {
	case "cloudflare":
		p.client, err = newCloudflare(env, p.Scope)
	case "digitalocean":
		p.client = newDigitalOcean(env)
	case "hetzner":
		p.client = newHetzner(env)
	case "linode":
		p.client = newLinode(env)
	case "namecheap":
		p.client = newNamecheap(env)
	case "pihole":
		// Pi-hole has no TXT records, so ownership tracking can't work.
		p.Force = true
		p.TTL, p.IgnoreTTL = 0, true
		p.client, err = newPihole(env)
	case "powerdns":
		p.TTL = cmp.Or(p.TTL, defaultTTL)
		p.client = newPowerDNS(env)
	case "rfc2136":
		p.TTL = cmp.Or(p.TTL, defaultTTL)
		p.client = newRFC2136(env)
	case "route53":
		p.TTL = cmp.Or(p.TTL, defaultTTL)
		p.client = newRoute53(env)
	case "scaleway":
		p.client = newScaleway(env)
	case "unifi":
		p.client, err = newUnifi(env)
	default:
		return nil, fmt.Errorf("unsupported type %q", typ)
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ParseTTL reads a TTL as seconds ("300") or a duration ("5m"). Empty means zero (auto).
// Values are clamped to 60s..24h, the range every supported provider accepts.
func ParseTTL(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	ttl, err := time.ParseDuration(value)
	if secs, serr := strconv.Atoi(value); serr == nil {
		ttl, err = time.Duration(secs)*time.Second, nil
	}
	if err != nil {
		return 0, fmt.Errorf("invalid TTL %q", value)
	}
	if ttl == 0 {
		return 0, nil
	}
	return min(max(ttl, time.Minute), 24*time.Hour).Round(time.Second), nil
}

func parseBool(value string, fallback bool) (bool, error) {
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseBool(value)
}
