// Package reconcile compares desired DNS states with actual provider states and applies the necessary changes.
package reconcile

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/libdns/libdns"
	"github.com/mizuchilabs/relayd/internal/dns"
	"github.com/mizuchilabs/relayd/internal/targets"
	"github.com/mizuchilabs/relayd/internal/util"
)

const txtPrefix = "relayd"

type recordKey struct {
	Type  string
	Name  string
	Value string
}

// Apply synchronizes desired DNS records and TXT ownership records with the provider.
func Apply(
	ctx context.Context,
	provider dns.Provider,
	instanceID, zone string,
	hosts []string,
	target targets.IPs,
) error {
	records, err := provider.Records(ctx, zone)
	if err != nil {
		return err
	}

	desired := desiredSet(hosts, zone)
	managed := managedSet(records, zone, instanceID)

	existingHosts := make(map[string]struct{})
	for _, r := range records {
		if r.Type == "A" || r.Type == "AAAA" || r.Type == "CNAME" {
			name := r.Name
			if name == "@" || name == "" {
				existingHosts[strings.TrimSuffix(util.WithDot(zone), ".")] = struct{}{}
			} else {
				absName := libdns.AbsoluteName(r.Name, util.WithDot(zone))
				existingHosts[strings.TrimSuffix(absName, ".")] = struct{}{}
			}
		}
	}

	var changes dns.ChangeSet
	var desiredRecords []dns.Record

	for fqdn := range desired {
		if !provider.Force() {
			if _, isManaged := managed[fqdn]; !isManaged {
				if _, exists := existingHosts[fqdn]; exists {
					slog.Warn(
						"Skipping unmanaged host with existing records (no relayd TXT record found)",
						"host",
						fqdn,
					)
					continue
				}
			}
		}

		rel := libdns.RelativeName(fqdn, util.WithDot(zone))
		// Order matters here
		if !provider.Force() {
			desiredRecords = append(desiredRecords, dns.Record{
				Type:  "TXT",
				Name:  txtName(rel),
				Value: txtValue(instanceID),
			})
		}
		if target.IPv4 != "" {
			desiredRecords = append(
				desiredRecords,
				dns.Record{Type: "A", Name: rel, Value: target.IPv4},
			)
		}
		if target.IPv6 != "" {
			desiredRecords = append(
				desiredRecords,
				dns.Record{Type: "AAAA", Name: rel, Value: target.IPv6},
			)
		}
	}

	existingMap := make(map[recordKey]dns.Record)
	for _, r := range records {
		key := recordKey{Type: r.Type, Name: r.Name, Value: r.Value}
		existingMap[key] = r
	}

	desiredMap := make(map[recordKey]dns.Record)
	for _, r := range desiredRecords {
		key := recordKey{Type: r.Type, Name: r.Name, Value: r.Value}
		desiredMap[key] = r
	}

	// Calculate Creates (Iterate slice to preserve TXT -> A/AAAA order)
	for _, r := range desiredRecords {
		key := recordKey{Type: r.Type, Name: r.Name, Value: r.Value}
		if _, exists := existingMap[key]; !exists {
			slog.Debug("Record to create", "type", r.Type, "name", r.Name, "value", r.Value)
			changes.Create = append(changes.Create, r)
			existingMap[key] = r // Add to existingMap temporarily so we don't duplicate creates
		}
	}

	// Calculate Deletes
	for _, r := range records {
		if r.Type != "A" && r.Type != "AAAA" && r.Type != "TXT" {
			continue
		}

		// Do not touch other TXT records (e.g., SPF, DKIM)
		if r.Type == "TXT" && !strings.HasPrefix(r.Name, txtPrefix) {
			continue
		}

		key := recordKey{Type: r.Type, Name: r.Name, Value: r.Value}
		if _, desired := desiredMap[key]; desired {
			continue
		}

		absName := libdns.AbsoluteName(r.Name, util.WithDot(zone))
		hostToCheck := strings.TrimSuffix(absName, ".")

		if r.Type == "TXT" && strings.HasPrefix(r.Name, txtPrefix) {
			if r.Name == txtPrefix {
				hostToCheck = strings.TrimSuffix(util.WithDot(zone), ".")
			} else if after, ok := strings.CutPrefix(r.Name, txtPrefix+"."); ok {
				hostToCheck = strings.TrimSuffix(libdns.AbsoluteName(after, util.WithDot(zone)), ".")
			}
		}

		shouldDelete := false
		if !provider.Force() {
			if _, isManaged := managed[hostToCheck]; isManaged {
				shouldDelete = true
			}
		} else {
			if _, isDesired := desired[hostToCheck]; !isDesired {
				shouldDelete = true
			}
		}

		if shouldDelete {
			slog.Debug("Record to delete",
				"type", r.Type, "name", r.Name, "value", r.Value,
				"host", hostToCheck, "force", provider.Force(),
			)
			changes.Delete = append(changes.Delete, r)
		}
	}

	// Sort Deletes: A/AAAA records first, TXT records LAST.
	// If deletion fails midway, we don't orphan A records without their owner TXT.
	sort.Slice(changes.Delete, func(i, j int) bool {
		if changes.Delete[i].Type != "TXT" && changes.Delete[j].Type == "TXT" {
			return true
		}
		if changes.Delete[i].Type == "TXT" && changes.Delete[j].Type != "TXT" {
			return false
		}
		return changes.Delete[i].Name < changes.Delete[j].Name
	})

	if len(changes.Create) == 0 && len(changes.Delete) == 0 {
		return nil
	}

	slog.Info("Applying changes",
		"add", len(changes.Create), "delete", len(changes.Delete),
		"provider", provider.Name(), "zone", zone,
	)
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

func managedSet(records []dns.Record, zone string, instanceID string) map[string]struct{} {
	out := make(map[string]struct{})
	zDot := util.WithDot(strings.ToLower(zone))

	for _, r := range records {
		val := strings.Trim(r.Value, "\"")

		// only claim management if the TXT value matches our specific instance
		if r.Type == "TXT" && val == txtValue(instanceID) {
			name := strings.Trim(strings.ToLower(r.Name), ".")
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

func txtValue(instanceID string) string {
	return "managed-by=relayd-" + instanceID
}
