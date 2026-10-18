package unifi

import (
	"testing"
	"time"

	"github.com/libdns/libdns"
	"github.com/stretchr/testify/require"
)

// Two SRV policies on the same domain that differ only by service must not match each other.
func TestDeleteSRVMatchesService(t *testing.T) {
	t.Parallel()
	ttl := int32(3600)
	f := &fakeUnifi{policies: []DNSPolicy{
		{
			ID: "a", Type: "SRV_RECORD", Domain: "app.home.lan", Service: "http", Protocol: "_tcp",
			ServerDomain: "srv.home.lan", Priority: 10, Weight: 5, Port: 8080, TTLSeconds: &ttl, Enabled: true,
		},
		{
			ID: "b", Type: "SRV_RECORD", Domain: "app.home.lan", Service: "https", Protocol: "_tcp",
			ServerDomain: "srv.home.lan", Priority: 10, Weight: 5, Port: 8080, TTLSeconds: &ttl, Enabled: true,
		},
	}}
	p := newTestProvider(t, f, testToken, testSite)

	_, err := p.DeleteRecords(t.Context(), testZone, []libdns.Record{
		libdns.RR{Type: "SRV", Name: "_https._tcp.app", Data: "10 5 8080 srv.home.lan", TTL: time.Hour},
	})
	require.NoError(t, err)

	got := f.snapshot()
	require.Len(t, got, 1)
	require.Equal(t, "a", got[0].ID, "deleting _https must leave the _http policy alone")
}

// Without an exact match, SetRecords must update the SRV policy with the same service, not a sibling.
func TestSetSRVFallbackMatchesService(t *testing.T) {
	t.Parallel()
	ttl := int32(3600)
	f := &fakeUnifi{policies: []DNSPolicy{
		{
			ID: "a", Type: "SRV_RECORD", Domain: "app.home.lan", Service: "http", Protocol: "_tcp",
			ServerDomain: "srv.home.lan", Priority: 10, Weight: 5, Port: 8080, TTLSeconds: &ttl, Enabled: true,
		},
		{
			ID: "b", Type: "SRV_RECORD", Domain: "app.home.lan", Service: "https", Protocol: "_tcp",
			ServerDomain: "srv.home.lan", Priority: 10, Weight: 5, Port: 8443, TTLSeconds: &ttl, Enabled: true,
		},
	}}
	p := newTestProvider(t, f, testToken, testSite)

	_, err := p.SetRecords(t.Context(), testZone, []libdns.Record{
		libdns.RR{Type: "SRV", Name: "_https._tcp.app", Data: "10 5 9443 srv.home.lan", TTL: time.Hour},
	})
	require.NoError(t, err)

	got := f.snapshot()
	require.Len(t, got, 2)
	require.Equal(t, uint16(8080), got[0].Port, "setting _https must leave the _http policy alone")
	require.Equal(t, uint16(9443), got[1].Port)
}
