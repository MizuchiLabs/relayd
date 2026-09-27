package pihole

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/libdns/libdns"
)

type config struct {
	Config struct {
		DNS struct {
			Hosts        []string `json:"hosts"`
			CNAMERecords []string `json:"cnameRecords"`
		} `json:"dns"`
	} `json:"config"`
}

// GetRecords returns the local DNS (A/AAAA) and CNAME entries inside zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	var hosts, cnames config
	if err := p.call(ctx, http.MethodGet, "/config/dns/hosts", &hosts); err != nil {
		return nil, err
	}
	if err := p.call(ctx, http.MethodGet, "/config/dns/cnameRecords", &cnames); err != nil {
		return nil, err
	}

	var records []libdns.Record
	for _, entry := range hosts.Config.DNS.Hosts {
		// "<ip> <name> [<name>...]"
		fields := strings.Fields(entry)
		if len(fields) < 2 {
			continue
		}
		typ := "A"
		if strings.Contains(fields[0], ":") {
			typ = "AAAA"
		}
		for _, name := range fields[1:] {
			if inZone(name, zone) {
				records = append(records, libdns.RR{Type: typ, Name: libdns.RelativeName(name, zone), Data: fields[0]})
			}
		}
	}
	for _, entry := range cnames.Config.DNS.CNAMERecords {
		// "<name>,<target>[,<ttl>]"
		parts := strings.Split(entry, ",")
		if len(parts) < 2 || !inZone(parts[0], zone) {
			continue
		}
		records = append(records, libdns.RR{Type: "CNAME", Name: libdns.RelativeName(parts[0], zone), Data: parts[1]})
	}
	return records, nil
}

func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	return p.each(ctx, http.MethodPut, zone, records)
}

func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	return p.each(ctx, http.MethodDelete, zone, records)
}

// each adds or removes local DNS entries one by one. Only A and AAAA are supported.
func (p *Provider) each(ctx context.Context, method, zone string, records []libdns.Record) ([]libdns.Record, error) {
	var done []libdns.Record
	for _, r := range records {
		rr := r.RR()
		if rr.Type != "A" && rr.Type != "AAAA" {
			return done, fmt.Errorf("pi-hole: unsupported record type %s", rr.Type)
		}
		host := strings.TrimSuffix(libdns.AbsoluteName(rr.Name, zone), ".")
		if err := p.call(ctx, method, "/config/dns/hosts/"+url.PathEscape(rr.Data+" "+host), nil); err != nil {
			return done, err
		}
		done = append(done, r)
	}
	return done, nil
}

func inZone(name, zone string) bool {
	name, zone = strings.TrimSuffix(name, "."), strings.TrimSuffix(zone, ".")
	return strings.EqualFold(name, zone) || strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(zone))
}
