package dns

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// Record is a normalized DNS record so values from different providers compare equal.
type Record struct {
	Type  string
	Name  string
	Value string
	TTL   time.Duration

	raw libdns.Record
}

// ChangeSet holds the changes for one zone. Update only changes the TTL of an existing record.
type ChangeSet struct {
	Create []Record
	Update []Record
	Delete []Record
}

func (c ChangeSet) Empty() bool {
	return len(c.Create) == 0 && len(c.Update) == 0 && len(c.Delete) == 0
}

func (p *Provider) Records(ctx context.Context, zone string) ([]Record, error) {
	records, err := p.client.GetRecords(ctx, zone)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(records))
	for _, r := range records {
		out = append(out, fromLibDNS(r))
	}
	return out, nil
}

// Apply creates before it deletes, so a host is never left without records mid-update.
func (p *Provider) Apply(ctx context.Context, zone string, changes ChangeSet) error {
	if len(changes.Create) > 0 {
		if _, err := p.client.AppendRecords(ctx, zone, toLibDNS(changes.Create)); err != nil {
			return err
		}
	}
	if len(changes.Update) > 0 {
		setter, ok := p.client.(libdns.RecordSetter)
		if !ok {
			return errors.New("provider can't update records")
		}
		if _, err := setter.SetRecords(ctx, zone, toLibDNS(changes.Update)); err != nil {
			return err
		}
	}
	if len(changes.Delete) > 0 {
		if _, err := p.client.DeleteRecords(ctx, zone, toLibDNS(changes.Delete)); err != nil {
			return err
		}
	}
	return nil
}

func fromLibDNS(record libdns.Record) Record {
	rr := record.RR()
	r := Record{
		Type:  strings.ToUpper(rr.Type),
		Name:  strings.ToLower(rr.Name),
		Value: rr.Data,
		TTL:   rr.TTL,
		raw:   record,
	}
	if r.Name == "" {
		r.Name = "@"
	}
	switch r.Type {
	case "TXT":
		r.Value = strings.Trim(r.Value, `"`)
	case "A", "AAAA":
		if ip, err := netip.ParseAddr(r.Value); err == nil {
			r.Value = ip.String()
		}
	}
	return r
}

// toLibDNS hands back the provider's own record when there is one, since some
// providers (UniFi) need the IDs they attached to it.
func toLibDNS(records []Record) []libdns.Record {
	out := make([]libdns.Record, 0, len(records))
	for _, r := range records {
		if r.raw != nil {
			out = append(out, r.raw)
			continue
		}
		rr := libdns.RR{Type: r.Type, Name: r.Name, Data: r.Value, TTL: r.TTL}
		if parsed, err := rr.Parse(); err == nil {
			out = append(out, parsed)
		} else {
			out = append(out, rr)
		}
	}
	return out
}
