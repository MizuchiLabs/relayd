package unifi

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/libdns/libdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testZone  = "home.lan"
	testToken = "token"
	testSite  = "site-1"
)

// fakeUnifi mimics the parts of the UniFi Network integration API relayd uses.
type fakeUnifi struct {
	mu        sync.Mutex
	policies  []DNSPolicy
	nextID    int
	listCalls int
}

func (f *fakeUnifi) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sites", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.MarshalWrite(w, map[string]any{"data": []map[string]string{
			{"id": "other", "name": "Office", "internalReference": "office"},
			{"id": testSite, "name": "Default", "internalReference": "default"},
		}})
	})
	mux.HandleFunc("GET /sites/{site}/dns/policies", func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		f.mu.Lock()
		defer f.mu.Unlock()
		f.listCalls++
		page := f.policies[min(offset, len(f.policies)):min(offset+limit, len(f.policies))]
		_ = json.MarshalWrite(w, map[string]any{"data": page, "totalCount": len(f.policies)})
	})
	mux.HandleFunc("POST /sites/{site}/dns/policies", func(w http.ResponseWriter, r *http.Request) {
		var pol DNSPolicy
		if err := json.UnmarshalRead(r.Body, &pol); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.nextID++
		pol.ID = fmt.Sprintf("p-%d", f.nextID)
		f.policies = append(f.policies, pol)
		_ = json.MarshalWrite(w, pol)
	})
	mux.HandleFunc("PUT /sites/{site}/dns/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		var pol DNSPolicy
		if err := json.UnmarshalRead(r.Body, &pol); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		i := f.index(r.PathValue("id"))
		if i < 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		pol.ID = r.PathValue("id")
		f.policies[i] = pol
		_ = json.MarshalWrite(w, pol)
	})
	mux.HandleFunc("DELETE /sites/{site}/dns/policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		i := f.index(r.PathValue("id"))
		if i < 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.policies = slices.Delete(f.policies, i, i+1)
		w.WriteHeader(http.StatusNoContent)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != testToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
			return
		}
		if site := r.PathValue("site"); site != "" && site != testSite {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (f *fakeUnifi) index(id string) int {
	return slices.IndexFunc(f.policies, func(p DNSPolicy) bool { return p.ID == id })
}

func (f *fakeUnifi) snapshot() []DNSPolicy {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.policies)
}

func newTestProvider(t *testing.T, f *fakeUnifi, token, site string) *Provider {
	t.Helper()
	srv := httptest.NewTLSServer(f.handler())
	t.Cleanup(srv.Close)
	return &Provider{Server: srv.URL, Token: token, Site: site, Insecure: true}
}

func TestRecordConversionRoundTrip(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		rr     libdns.RR
		domain string
		typ    string
	}{
		"A": {
			libdns.RR{Type: "A", Name: "app", Data: "10.0.0.1", TTL: 5 * time.Minute},
			"app.home.lan",
			"A_RECORD",
		},
		"A apex": {libdns.RR{Type: "A", Name: "@", Data: "10.0.0.2", TTL: time.Minute}, "home.lan", "A_RECORD"},
		"AAAA": {
			libdns.RR{Type: "AAAA", Name: "app", Data: "fd00::1", TTL: time.Minute},
			"app.home.lan",
			"AAAA_RECORD",
		},
		"CNAME": {
			libdns.RR{Type: "CNAME", Name: "www", Data: "app.home.lan", TTL: time.Minute},
			"www.home.lan",
			"CNAME_RECORD",
		},
		"TXT no TTL": {
			libdns.RR{Type: "TXT", Name: "_relayd.app", Data: "owner=relayd"},
			"_relayd.app.home.lan",
			"TXT_RECORD",
		},
		"MX": {
			libdns.RR{Type: "MX", Name: "@", Data: "10 mail.home.lan", TTL: time.Hour},
			"home.lan",
			"MX_RECORD",
		},
		"SRV": {
			libdns.RR{Type: "SRV", Name: "_http._tcp.app", Data: "10 5 8080 app.home.lan", TTL: time.Hour},
			"app.home.lan", "SRV_RECORD",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pol, err := libdnsToPolicy(tc.rr, testZone)
			require.NoError(t, err)
			assert.Equal(t, tc.domain, pol.Domain)
			assert.Equal(t, tc.typ, pol.Type)
			assert.True(t, pol.Enabled)

			back, err := policyToLibdns(pol, testZone)
			require.NoError(t, err)
			assert.Equal(t, tc.rr, back.RR())
		})
	}
}

func TestRecordConversionKeepsQualifiedName(t *testing.T) {
	t.Parallel()
	pol, err := libdnsToPolicy(libdns.RR{Type: "A", Name: "app.home.lan", Data: "10.0.0.1"}, testZone)
	require.NoError(t, err)
	assert.Equal(t, "app.home.lan", pol.Domain, "an already qualified name must not get the zone twice")
}

func TestRecordConversionUnsupported(t *testing.T) {
	t.Parallel()
	_, err := libdnsToPolicy(libdns.RR{Type: "CAA", Name: "@", Data: `0 issue "letsencrypt.org"`}, testZone)
	require.Error(t, err)
	_, err = policyToLibdns(DNSPolicy{Type: "FORWARD_DOMAIN", Domain: testZone}, testZone)
	require.Error(t, err)
}

func TestGetRecordsPages(t *testing.T) {
	t.Parallel()
	f := &fakeUnifi{}
	for i := range 250 {
		f.policies = append(f.policies, DNSPolicy{
			ID:          fmt.Sprintf("p-%d", i),
			Type:        "A_RECORD",
			Domain:      fmt.Sprintf("host%d.home.lan", i),
			IPv4Address: "10.0.0.1",
			Enabled:     true,
		})
	}
	f.policies = append(f.policies, DNSPolicy{ID: "fwd", Type: "FORWARD_DOMAIN", Domain: testZone})
	p := newTestProvider(t, f, testToken, "default")

	records, err := p.GetRecords(t.Context(), testZone+".")
	require.NoError(t, err)
	require.Len(t, records, 250, "every page is read and unsupported types are skipped")
	assert.Equal(t, 3, f.listCalls)

	first, ok := records[0].(unifiRecord)
	require.True(t, ok, "records carry their policy ID")
	assert.Equal(t, "p-0", first.ID)
	assert.Equal(t, libdns.RR{Type: "A", Name: "host0", Data: "10.0.0.1"}, first.RR())
}

func TestAppendSetDelete(t *testing.T) {
	t.Parallel()
	f := &fakeUnifi{}
	p := newTestProvider(t, f, testToken, testSite)
	rec := libdns.RR{Type: "A", Name: "app", Data: "10.0.0.1", TTL: time.Minute}

	added, err := p.AppendRecords(t.Context(), testZone, []libdns.Record{rec})
	require.NoError(t, err)
	require.Len(t, added, 1)
	assert.Equal(t, "p-1", added[0].(unifiRecord).ID)

	rec.TTL = 10 * time.Minute
	_, err = p.SetRecords(t.Context(), testZone, []libdns.Record{rec})
	require.NoError(t, err)
	got := f.snapshot()
	require.Len(t, got, 1, "set without an ID must update the existing policy, not add one")
	assert.Equal(t, "p-1", got[0].ID)
	assert.Equal(t, int32(600), *got[0].TTLSeconds)

	_, err = p.DeleteRecords(t.Context(), testZone, []libdns.Record{rec})
	require.NoError(t, err)
	assert.Empty(t, f.snapshot())

	_, err = p.DeleteRecords(t.Context(), testZone, []libdns.Record{rec})
	require.NoError(t, err, "deleting a missing record is not an error")
}

func TestSetRecordsUsesID(t *testing.T) {
	t.Parallel()
	f := &fakeUnifi{policies: []DNSPolicy{
		{ID: "a", Type: "A_RECORD", Domain: "app.home.lan", IPv4Address: "10.0.0.1", Enabled: true},
		{ID: "b", Type: "A_RECORD", Domain: "app.home.lan", IPv4Address: "10.0.0.2", Enabled: true},
	}}
	p := newTestProvider(t, f, testToken, testSite)

	rec := unifiRecord{Record: libdns.RR{Type: "A", Name: "app", Data: "10.0.0.2", TTL: time.Hour}, ID: "b"}
	_, err := p.SetRecords(t.Context(), testZone, []libdns.Record{rec})
	require.NoError(t, err)

	got := f.snapshot()
	assert.Nil(t, got[0].TTLSeconds, "policy a must be untouched")
	require.NotNil(t, got[1].TTLSeconds)
	assert.Equal(t, int32(3600), *got[1].TTLSeconds)
}

// reconcile sends TTL updates without IDs, so SetRecords has to find the policy itself.
func TestSetRecordsWithoutIDMatchesValue(t *testing.T) {
	t.Parallel()
	f := &fakeUnifi{policies: []DNSPolicy{
		{ID: "a", Type: "A_RECORD", Domain: "app.home.lan", IPv4Address: "10.0.0.1", Enabled: true},
		{ID: "b", Type: "A_RECORD", Domain: "app.home.lan", IPv4Address: "10.0.0.2", Enabled: true},
	}}
	p := newTestProvider(t, f, testToken, testSite)

	rec := libdns.RR{Type: "A", Name: "app", Data: "10.0.0.2", TTL: time.Hour}
	_, err := p.SetRecords(t.Context(), testZone, []libdns.Record{rec})
	require.NoError(t, err)

	got := f.snapshot()
	require.Len(t, got, 2)
	assert.Equal(
		t,
		DNSPolicy{ID: "a", Type: "A_RECORD", Domain: "app.home.lan", IPv4Address: "10.0.0.1", Enabled: true},
		got[0],
		"policy a must be untouched",
	)
	require.NotNil(t, got[1].TTLSeconds)
	assert.Equal(t, int32(3600), *got[1].TTLSeconds)
}

func TestDeleteRecordsMatchesValue(t *testing.T) {
	t.Parallel()
	f := &fakeUnifi{policies: []DNSPolicy{
		{ID: "a", Type: "A_RECORD", Domain: "app.home.lan", IPv4Address: "10.0.0.1", Enabled: true},
		{ID: "b", Type: "A_RECORD", Domain: "app.home.lan", IPv4Address: "10.0.0.2", Enabled: true},
	}}
	p := newTestProvider(t, f, testToken, testSite)

	_, err := p.DeleteRecords(
		t.Context(),
		testZone,
		[]libdns.Record{libdns.RR{Type: "A", Name: "app", Data: "10.0.0.2"}},
	)
	require.NoError(t, err)

	got := f.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].ID)
}

func TestSiteNotFound(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, &fakeUnifi{}, testToken, "nope")
	_, err := p.GetRecords(t.Context(), testZone)
	assert.ErrorContains(t, err, "site 'nope' not found")
}

func TestWrongToken(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, &fakeUnifi{}, "wrong", testSite)
	_, err := p.GetRecords(t.Context(), testZone)
	assert.ErrorContains(t, err, "status 401")
}

func TestVerifiesCertWhenNotInsecure(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, &fakeUnifi{}, testToken, testSite)
	p.Insecure = false
	_, err := p.GetRecords(t.Context(), testZone)
	assert.ErrorContains(t, err, "certificate")
}
