package dns

import (
	"context"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// libDNSClient defines the interface expected from a libdns provider.
type libDNSClient interface {
	GetRecords(ctx context.Context, zone string) ([]libdns.Record, error)
	AppendRecords(
		ctx context.Context,
		zone string,
		records []libdns.Record,
	) ([]libdns.Record, error)
	SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error)
	DeleteRecords(
		ctx context.Context,
		zone string,
		records []libdns.Record,
	) ([]libdns.Record, error)
}

// wrapper provides a uniform interface to a libDNSClient.
type wrapper struct {
	scope  string
	zones  []string
	client libDNSClient
}

func (w *wrapper) Scope() string {
	return w.scope
}

func (w *wrapper) Zones() []string {
	return append([]string(nil), w.zones...)
}

func (w *wrapper) Records(ctx context.Context, zone string) ([]Record, error) {
	records, err := w.client.GetRecords(ctx, zone)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(records))
	for _, r := range records {
		out = append(out, fromLibDNS(r))
	}
	return out, nil
}

func (w *wrapper) Apply(ctx context.Context, zone string, changes ChangeSet) error {
	create := toLibDNS(changes.Create)
	update := toLibDNS(changes.Update)
	deleteSet := toLibDNS(changes.Delete)

	if len(create) > 0 {
		if _, err := w.client.AppendRecords(ctx, zone, create); err != nil {
			return err
		}
	}
	if len(update) > 0 {
		if _, err := w.client.SetRecords(ctx, zone, update); err != nil {
			return err
		}
	}
	if len(deleteSet) > 0 {
		if _, err := w.client.DeleteRecords(ctx, zone, deleteSet); err != nil {
			return err
		}
	}

	return nil
}

func toLibDNS(records []Record) []libdns.Record {
	out := make([]libdns.Record, 0, len(records))
	for _, r := range records {
		out = append(out, libdns.RR{
			Type: strings.ToUpper(r.Type),
			Name: r.Name,
			Data: r.Value,
			TTL:  r.TTL,
		})
	}
	return out
}

func fromLibDNS(record libdns.Record) Record {
	rr := record.RR()
	return Record{
		Type:  rr.Type,
		Name:  rr.Name,
		Value: rr.Data,
		TTL:   sanitizeTTL(rr.TTL),
	}
}

func sanitizeTTL(ttl time.Duration) time.Duration {
	if ttl < 0 {
		return 0
	}
	return ttl
}
