package dns

import (
	"context"
	"strings"

	"github.com/libdns/libdns"
	"github.com/mizuchilabs/relayd/internal/config"
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
	name   string
	scope  string
	zones  []string
	force  bool
	client libDNSClient
}

func newWrapper(cfg config.Provider, client libDNSClient) *wrapper {
	return &wrapper{
		name:   cfg.Name,
		scope:  cfg.Scope,
		zones:  append([]string(nil), cfg.Zones...),
		force:  cfg.Force,
		client: client,
	}
}

func (w *wrapper) Name() string {
	return w.name
}

func (w *wrapper) Scope() string {
	return w.scope
}

func (w *wrapper) Zones() []string {
	return append([]string(nil), w.zones...)
}

func (w *wrapper) Force() bool {
	return w.force
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
	createSet := toLibDNS(changes.Create)
	updateSet := toLibDNS(changes.Update)
	deleteSet := toLibDNS(changes.Delete)

	if len(createSet) > 0 {
		if _, err := w.client.AppendRecords(ctx, zone, createSet); err != nil {
			return err
		}
	}
	if len(updateSet) > 0 {
		if _, err := w.client.SetRecords(ctx, zone, updateSet); err != nil {
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
		if r.Original != nil {
			out = append(out, r.Original)
		} else {
			rr := libdns.RR{
				Type: strings.ToUpper(r.Type),
				Name: r.Name,
				Data: r.Value,
			}
			if parsed, err := rr.Parse(); err == nil && parsed != nil {
				out = append(out, parsed)
			} else {
				out = append(out, rr)
			}
		}
	}
	return out
}

func fromLibDNS(record libdns.Record) Record {
	rr := record.RR()
	return Record{
		Type:     rr.Type,
		Name:     rr.Name,
		Value:    rr.Data,
		Original: record,
	}
}
