package unifi

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

type Provider struct {
	client *Client
	Server string `json:"server,omitempty"`
	Token  string `json:"token,omitempty"`
	Site   string `json:"site,omitempty"`
}

func (p *Provider) getClient() *Client {
	if p.client == nil {
		p.client = &Client{BaseURL: p.Server, Token: p.Token, Site: p.Site}
	}
	return p.client
}

func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	c := p.getClient()
	policies, err := c.getRecords(ctx, zone)
	if err != nil {
		return nil, err
	}

	var records []libdns.Record
	zoneTrimmed := strings.TrimSuffix(zone, ".")

	for _, pol := range policies {
		r, err := policyToLibdns(pol, zoneTrimmed)
		if err != nil {
			continue // skip unsupported
		}
		records = append(records, r)
	}

	return records, nil
}

func (p *Provider) AppendRecords(
	ctx context.Context,
	zone string,
	records []libdns.Record,
) ([]libdns.Record, error) {
	c := p.getClient()
	zoneTrimmed := strings.TrimSuffix(zone, ".")
	var added []libdns.Record

	for _, r := range records {
		pol, err := libdnsToPolicy(r, zoneTrimmed)
		if err != nil {
			return added, err
		}

		_, err = c.createRecord(ctx, pol)
		if err != nil {
			return added, err
		}

		added = append(added, r)
	}

	return added, nil
}

func (p *Provider) SetRecords(
	ctx context.Context,
	zone string,
	records []libdns.Record,
) ([]libdns.Record, error) {
	c := p.getClient()
	zoneTrimmed := strings.TrimSuffix(zone, ".")
	var set []libdns.Record

	existing, err := c.getRecords(ctx, zone)
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		pol, err := libdnsToPolicy(r, zoneTrimmed)
		if err != nil {
			return set, err
		}

		// Find existing
		var existingID string
		for _, e := range existing {
			if e.Domain == pol.Domain && e.Type == pol.Type {
				existingID = e.ID
				break
			}
		}

		if existingID != "" {
			_, err = c.updateRecord(ctx, existingID, pol)
			if err != nil {
				return set, err
			}
		} else {
			_, err = c.createRecord(ctx, pol)
			if err != nil {
				return set, err
			}
		}

		set = append(set, r)
	}

	return set, nil
}

func (p *Provider) DeleteRecords(
	ctx context.Context,
	zone string,
	records []libdns.Record,
) ([]libdns.Record, error) {
	c := p.getClient()
	var deleted []libdns.Record
	zoneTrimmed := strings.TrimSuffix(zone, ".")

	existing, err := c.getRecords(ctx, zone)
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		pol, err := libdnsToPolicy(r, zoneTrimmed)
		if err != nil {
			return deleted, err
		}

		var existingID string
		for _, e := range existing {
			// Now we check that the actual values match, not just the name/type
			if isExactMatch(e, pol) {
				existingID = e.ID
				break
			}
		}

		if existingID != "" {
			if err := c.deleteRecord(ctx, existingID); err != nil {
				return deleted, err
			}
		}
		deleted = append(deleted, r)
	}

	return deleted, nil
}

func policyToLibdns(policy DNSPolicy, zone string) (libdns.Record, error) {
	ttl := time.Duration(policy.TTLSeconds) * time.Second

	name := policy.Domain
	if name == zone {
		name = "@"
	} else if before, ok := strings.CutSuffix(name, "."+zone); ok {
		name = before
	}

	rr := libdns.RR{
		Name: name,
		TTL:  ttl,
	}

	switch policy.Type {
	case "A_RECORD":
		rr.Type = "A"
		ip, _ := netip.ParseAddr(policy.IPv4Address)
		rr.Data = ip.String()
	case "AAAA_RECORD":
		rr.Type = "AAAA"
		ip, _ := netip.ParseAddr(policy.IPv6Address)
		rr.Data = ip.String()
	case "CNAME_RECORD":
		rr.Type = "CNAME"
		rr.Data = policy.TargetDomain
	case "TXT_RECORD":
		rr.Type = "TXT"
		rr.Data = policy.Text
	case "MX_RECORD":
		rr.Type = "MX"
		rr.Data = fmt.Sprintf("%d %s", policy.Priority, policy.MailServerDomain)
	case "SRV_RECORD":
		rr.Type = "SRV"
		rr.Data = fmt.Sprintf(
			"%d %d %d %s",
			policy.Priority,
			policy.Weight,
			policy.Port,
			policy.ServerDomain,
		)

		nameSrv := policy.Domain
		if nameSrv == zone {
			nameSrv = "@"
		} else if before, ok := strings.CutSuffix(nameSrv, "."+zone); ok {
			nameSrv = before
		}
		rr.Name = fmt.Sprintf(
			"_%s._%s.%s",
			policy.Service,
			strings.TrimPrefix(policy.Protocol, "_"),
			nameSrv,
		)
	default:
		return nil, fmt.Errorf("unsupported record type: %s", policy.Type)
	}

	// For compatibility with libdns specific types
	parsed, err := rr.Parse()
	if err == nil && parsed != nil {
		return parsed, nil
	}
	return rr, nil
}

func libdnsToPolicy(record libdns.Record, zone string) (DNSPolicy, error) {
	r := record.RR()
	domain := r.Name
	if domain == "@" || domain == "" {
		domain = zone
	} else if !strings.HasSuffix(domain, "."+zone) {
		domain = domain + "." + zone
	}

	ttl := int32(r.TTL.Seconds())

	switch r.Type {
	case "A":
		return DNSPolicy{
			Type:        "A_RECORD",
			Domain:      domain,
			IPv4Address: r.Data,
			TTLSeconds:  ttl,
			Enabled:     true,
		}, nil
	case "AAAA":
		return DNSPolicy{
			Type:        "AAAA_RECORD",
			Domain:      domain,
			IPv6Address: r.Data,
			TTLSeconds:  ttl,
			Enabled:     true,
		}, nil
	case "CNAME":
		return DNSPolicy{
			Type:         "CNAME_RECORD",
			Domain:       domain,
			TargetDomain: r.Data,
			TTLSeconds:   ttl,
			Enabled:      true,
		}, nil
	case "TXT":
		return DNSPolicy{
			Type:       "TXT_RECORD",
			Domain:     domain,
			Text:       r.Data,
			TTLSeconds: ttl,
			Enabled:    true,
		}, nil
	case "MX":
		// Parse from Data (preference target)
		var priority uint16
		var target string
		_, _ = fmt.Sscanf(r.Data, "%d %s", &priority, &target)

		return DNSPolicy{
			Type:             "MX_RECORD",
			Domain:           domain,
			MailServerDomain: target,
			Priority:         priority,
			TTLSeconds:       ttl,
			Enabled:          true,
		}, nil
	case "SRV":
		// Parse from Data (priority weight port target)
		var priority, weight, port uint16
		var target string
		_, _ = fmt.Sscanf(r.Data, "%d %d %d %s", &priority, &weight, &port, &target)

		// Parse service and protocol from Name
		parts := strings.SplitN(r.Name, ".", 3)
		if len(parts) < 2 {
			return DNSPolicy{}, fmt.Errorf("invalid SRV name format: %s", r.Name)
		}

		name := "@"
		if len(parts) == 3 {
			name = parts[2]
		}
		if name == "@" || name == "" {
			name = zone
		} else if !strings.HasSuffix(name, "."+zone) {
			name = name + "." + zone
		}

		return DNSPolicy{
			Type:         "SRV_RECORD",
			Domain:       name,
			ServerDomain: target,
			Service:      strings.TrimPrefix(parts[0], "_"),
			Protocol:     "_" + strings.TrimPrefix(parts[1], "_"),
			Port:         port,
			Weight:       weight,
			Priority:     priority,
			TTLSeconds:   ttl,
			Enabled:      true,
		}, nil
	default:
		return DNSPolicy{}, fmt.Errorf("unsupported record type: %s", r.Type)
	}
}

// isExactMatch checks if two DNSPolicy records represent the exact same DNS entry
// (ignoring ID, TTL, Enabled flags, etc.).
func isExactMatch(existing, target DNSPolicy) bool {
	if existing.Domain != target.Domain || existing.Type != target.Type {
		return false
	}

	switch target.Type {
	case "A_RECORD":
		return existing.IPv4Address == target.IPv4Address
	case "AAAA_RECORD":
		return existing.IPv6Address == target.IPv6Address
	case "CNAME_RECORD":
		return existing.TargetDomain == target.TargetDomain
	case "TXT_RECORD":
		// Note: Depending on the UniFi API, it might return TXT strings wrapped in double quotes.
		// If so, you may need to compare them like: strings.Trim(existing.Text, `"`) == strings.Trim(target.Text, `"`)
		return existing.Text == target.Text
	case "MX_RECORD":
		return existing.MailServerDomain == target.MailServerDomain &&
			existing.Priority == target.Priority
	case "SRV_RECORD":
		return existing.ServerDomain == target.ServerDomain &&
			existing.Priority == target.Priority &&
			existing.Weight == target.Weight &&
			existing.Port == target.Port
	default:
		return false
	}
}
