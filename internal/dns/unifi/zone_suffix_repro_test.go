package unifi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The UniFi filter domain.like('*home.lan') also matches "notahome.lan", which must not leak into home.lan.
func TestGetRecordsIgnoresNamesOutsideZone(t *testing.T) {
	t.Parallel()
	f := &fakeUnifi{policies: []DNSPolicy{
		{ID: "in", Type: "A_RECORD", Domain: "app.home.lan", IPv4Address: "10.0.0.1", Enabled: true},
		{ID: "out", Type: "A_RECORD", Domain: "notahome.lan", IPv4Address: "10.0.0.9", Enabled: true},
	}}
	p := newTestProvider(t, f, testToken, "default")

	records, err := p.GetRecords(t.Context(), testZone+".")
	require.NoError(t, err)
	for _, r := range records {
		assert.True(
			t,
			r.RR().Name == "app" || r.RR().Name == "@" || r.RR().Name == "",
			"record outside zone: %+v",
			r.RR(),
		)
	}
	assert.Len(t, records, 1)
}
