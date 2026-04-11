// Package reconcile compares desired DNS states with actual provider states and applies the necessary changes.
package reconcile

import (
	"context"
	"strings"
	"time"

	"github.com/libdns/libdns"
	"github.com/mizuchilabs/relayd/internal/config"
	"github.com/mizuchilabs/relayd/internal/dns"
	"github.com/mizuchilabs/relayd/internal/targets"
	"github.com/mizuchilabs/relayd/internal/util"
)

const txtPrefix = "_relayd"

// Apply synchronizes desired DNS records and TXT ownership records with the provider.
func Apply(
	ctx context.Context,
	provider dns.Provider,
	zone string,
	cfg config.Config,
	hosts []string,
	target targets.IPs,
) error {
	records, err := provider.Records(ctx, zone)
	if err != nil {
		return err
	}

	desired := desiredSet(hosts, zone)
	managed := managedSet(records, zone)

	var changes dns.ChangeSet
	ttl := 300 * time.Second

	for fqdn := range desired {
		rel := libdns.RelativeName(fqdn, util.WithDot(zone))

		if target.IPv4 != "" {
			changes.Update = append(
				changes.Update,
				dns.Record{Type: "A", Name: rel, Value: target.IPv4, TTL: ttl},
			)
		}
		if target.IPv6 != "" {
			changes.Update = append(
				changes.Update,
				dns.Record{Type: "AAAA", Name: rel, Value: target.IPv6, TTL: ttl},
			)
		}

		if !cfg.Force {
			changes.Update = append(
				changes.Update,
				dns.Record{
					Type:  "TXT",
					Name:  txtName(rel),
					Value: "relayd",
					TTL:   ttl,
				},
			)
		}
	}

	if !cfg.Force {
		for fqdn := range managed {
			rel := libdns.RelativeName(fqdn, util.WithDot(zone))
			if _, ok := desired[fqdn]; !ok {
				changes.Delete = append(changes.Delete,
					dns.Record{Type: "A", Name: rel},
					dns.Record{Type: "AAAA", Name: rel},
					dns.Record{Type: "TXT", Name: txtName(rel)},
				)
			} else {
				if target.IPv4 == "" {
					changes.Delete = append(changes.Delete, dns.Record{Type: "A", Name: rel})
				}
				if target.IPv6 == "" {
					changes.Delete = append(changes.Delete, dns.Record{Type: "AAAA", Name: rel})
				}
			}
		}
	}

	if len(changes.Update) == 0 && len(changes.Delete) == 0 && len(changes.Create) == 0 {
		return nil
	}

	return provider.Apply(ctx, zone, changes)
}

func desiredSet(hosts []string, zone string) map[string]struct{} {
	out := make(map[string]struct{})
	zDot := util.WithDot(strings.ToLower(zone))
	for _, h := range hosts {
		full := util.WithDot(h)
		if full == zDot || strings.HasSuffix(full, "."+zDot) {
			out[strings.TrimSuffix(full, ".")] = struct{}{}
		}
	}
	return out
}

func managedSet(records []dns.Record, zone string) map[string]struct{} {
	out := make(map[string]struct{})
	zDot := util.WithDot(zone)

	for _, r := range records {
		if r.Type == "TXT" && r.Value == "relayd" {
			name := strings.Trim(r.Name, ".")
			if name == txtPrefix {
				out[strings.TrimSuffix(zDot, ".")] = struct{}{}
			} else if after, ok := strings.CutPrefix(name, txtPrefix+"."); ok {
				rel := after
				out[strings.TrimSuffix(libdns.AbsoluteName(rel, zDot), ".")] = struct{}{}
			}
		}
	}
	return out
}

func txtName(rel string) string {
	if rel == "" || rel == "@" {
		return txtPrefix
	}
	return txtPrefix + "." + rel
}
