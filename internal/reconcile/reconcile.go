// Package reconcile works out which DNS records to create and delete in a zone.
//
// Without force, relayd owns a host only while a TXT record "relayd.<host>" with
// its instance value exists, and never touches anything else.
// With force, relayd owns every A and AAAA record in the zone.
package reconcile

import (
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/mizuchilabs/relayd/internal/dns"
	"github.com/mizuchilabs/relayd/internal/targets"
)

const txtPrefix = "relayd"

// Desired is what relayd wants to publish in one zone.
type Desired struct {
	Zone     string
	Instance string
	// Hosts maps each hostname to its TTL. Zero leaves the TTL to the provider.
	Hosts map[string]time.Duration
	IPs   targets.IPs
	Force bool
}

type key struct {
	Type, Name, Value string
}

// Plan diffs existing records against the desired state. Records must be normalized as dns.Provider.Records returns them.
func Plan(d Desired, existing []dns.Record) dns.ChangeSet {
	zone := strings.ToLower(strings.TrimSuffix(d.Zone, "."))
	owner := "managed-by=relayd-" + d.Instance

	owned := map[string]bool{}
	taken := map[string]bool{}
	cname := map[string]bool{}
	for _, r := range existing {
		switch r.Type {
		case "A", "AAAA":
			taken[fqdn(r.Name, zone)] = true
		case "CNAME":
			taken[fqdn(r.Name, zone)] = true
			cname[fqdn(r.Name, zone)] = true
		case "TXT":
			if host, ok := txtHost(r.Name, zone); ok && r.Value == owner {
				owned[host] = true
			}
		}
	}

	// TXT goes first so a host is never published without its owner record.
	var want []dns.Record
	for _, host := range hostsInZone(d.Hosts, zone) {
		switch {
		case cname[host]:
			slog.Warn("Skipping host with a CNAME record", "host", host)
			continue
		case !d.Force && taken[host] && !owned[host]:
			slog.Warn("Skipping host with records relayd does not own", "host", host)
			continue
		}
		name, ttl := relative(host, zone), d.Hosts[host]
		if !d.Force {
			want = append(want, dns.Record{Type: "TXT", Name: txtName(name), Value: owner, TTL: ttl})
		}
		if d.IPs.IPv4 != "" {
			want = append(want, dns.Record{Type: "A", Name: name, Value: d.IPs.IPv4, TTL: ttl})
		}
		if d.IPs.IPv6 != "" {
			want = append(want, dns.Record{Type: "AAAA", Name: name, Value: d.IPs.IPv6, TTL: ttl})
		}
	}

	have := map[key]dns.Record{}
	for _, r := range existing {
		have[keyOf(r)] = r
	}
	wanted := map[key]bool{}
	var changes dns.ChangeSet
	for _, r := range want {
		wanted[keyOf(r)] = true
		cur, ok := have[keyOf(r)]
		switch {
		case !ok:
			changes.Create = append(changes.Create, r)
		// A zero TTL means auto, which each provider reports differently, so only explicit TTLs are compared.
		// TXT is skipped because some providers (UniFi) keep no TTL on it.
		case r.Type != "TXT" && r.TTL != 0 && cur.TTL != r.TTL:
			changes.Update = append(changes.Update, r)
		}
	}

	// A and AAAA go before TXT so a failed delete never orphans an address record.
	var txts []dns.Record
	for _, r := range existing {
		if wanted[keyOf(r)] {
			continue
		}
		switch r.Type {
		case "A", "AAAA":
			if d.Force || owned[fqdn(r.Name, zone)] {
				changes.Delete = append(changes.Delete, r)
			}
		case "TXT":
			if _, ok := txtHost(r.Name, zone); ok && r.Value == owner {
				txts = append(txts, r)
			}
		}
	}
	changes.Delete = append(changes.Delete, txts...)
	return changes
}

func keyOf(r dns.Record) key {
	return key{r.Type, r.Name, r.Value}
}

// hostsInZone returns the sorted hosts that belong to zone.
func hostsInZone(hosts map[string]time.Duration, zone string) []string {
	var out []string
	for h := range hosts {
		if h == zone || strings.HasSuffix(h, "."+zone) {
			out = append(out, h)
		}
	}
	slices.Sort(out)
	return out
}

func fqdn(name, zone string) string {
	switch {
	case name == "@" || name == "":
		return zone
	case strings.HasSuffix(name, "."):
		return strings.TrimSuffix(name, ".")
	default:
		return name + "." + zone
	}
}

func relative(host, zone string) string {
	if host == zone {
		return "@"
	}
	return strings.TrimSuffix(host, "."+zone)
}

func txtName(name string) string {
	if name == "@" {
		return txtPrefix
	}
	return txtPrefix + "." + name
}

// txtHost returns the host an ownership TXT record name points at.
func txtHost(name, zone string) (string, bool) {
	if name == txtPrefix {
		return zone, true
	}
	if rest, ok := strings.CutPrefix(name, txtPrefix+"."); ok {
		return fqdn(rest, zone), true
	}
	return "", false
}
