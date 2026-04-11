package pihole

import (
	"context"
	"strings"

	"github.com/libdns/libdns"
)

// Provider implements the libdns interfaces for Pi-hole
type Provider struct {
	client   *Client
	Server   string `json:"server,omitempty"`
	Password string `json:"password,omitempty"`
}

func (p *Provider) getClient() *Client {
	if p.client == nil {
		p.client = &Client{Server: p.Server, Password: p.Password}
	}
	return p.client
}

func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	c := p.getClient()
	var records []libdns.Record
	zoneTrimmed := strings.TrimSuffix(zone, ".")

	// Get A/AAAA records
	dnsData, err := c.getRecords(ctx, "customdns")
	if err != nil {
		return nil, err
	}
	for _, row := range dnsData {
		if len(row) >= 2 {
			domain := row[0]
			ip := row[1]
			if strings.HasSuffix(domain, zoneTrimmed) {
				rel := libdns.RelativeName(domain+".", zone)
				typ := "A"
				if strings.Contains(ip, ":") {
					typ = "AAAA"
				}
				records = append(records, libdns.RR{
					Type: typ,
					Name: rel,
					Data: ip,
				})
			}
		}
	}

	// Get CNAME records
	cnameData, err := c.getRecords(ctx, "customcname")
	if err != nil {
		return nil, err
	}
	for _, row := range cnameData {
		if len(row) >= 2 {
			domain := row[0]
			target := row[1]
			if strings.HasSuffix(domain, zoneTrimmed) {
				rel := libdns.RelativeName(domain+".", zone)
				records = append(records, libdns.RR{
					Type: "CNAME",
					Name: rel,
					Data: target,
				})
			}
		}
	}

	return records, nil
}

func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	c := p.getClient()
	var added []libdns.Record
	for _, r := range records {
		rr := r.RR()
		abs := libdns.AbsoluteName(rr.Name, zone)
		abs = strings.TrimSuffix(abs, ".")
		rec := libdns.RR{
			Type: rr.Type,
			Name: abs,
			Data: rr.Data,
		}
		if err := c.addRecord(ctx, rec); err != nil {
			return added, err
		}
		added = append(added, r)
	}
	return added, nil
}

func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	return p.AppendRecords(ctx, zone, records)
}

func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	c := p.getClient()
	var deleted []libdns.Record
	for _, r := range records {
		rr := r.RR()
		abs := libdns.AbsoluteName(rr.Name, zone)
		abs = strings.TrimSuffix(abs, ".")
		rec := libdns.RR{
			Type: rr.Type,
			Name: abs,
			Data: rr.Data,
		}
		if err := c.deleteRecord(ctx, rec); err != nil {
			return deleted, err
		}
		deleted = append(deleted, r)
	}
	return deleted, nil
}
