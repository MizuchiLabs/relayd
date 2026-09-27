package dns

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/libdns/cloudflare"
	"github.com/libdns/libdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProviders(t *testing.T) {
	t.Setenv("RELAYD_PROVIDER_CF_TYPE", "cloudflare")
	t.Setenv("RELAYD_PROVIDER_CF_ZONES", " Example.com , example.org,")
	t.Setenv("RELAYD_PROVIDER_PI_HOLE_TYPE", "pihole")
	t.Setenv("RELAYD_PROVIDER_PI_HOLE_SCOPE", "local")
	t.Setenv("RELAYD_PROVIDER_PI_HOLE_ZONES", "home.lan")

	providers, err := LoadProviders()
	require.NoError(t, err)
	require.Len(t, providers, 2)

	cf := providers[0]
	assert.Equal(t, "CF", cf.Name)
	assert.Equal(t, "public", cf.Scope)
	assert.Equal(t, []string{"example.com", "example.org"}, cf.Zones)
	assert.False(t, cf.Force)

	pi := providers[1]
	assert.Equal(t, "PI_HOLE", pi.Name)
	assert.Equal(t, "local", pi.Scope)
	assert.True(t, pi.Force, "pihole always runs in force mode")
}

func TestNewProviderErrors(t *testing.T) {
	t.Parallel()
	tests := map[string]map[string]string{
		"unknown type":  {"TYPE": "nope", "ZONES": "example.com"},
		"missing zones": {"TYPE": "hetzner"},
		"bad scope":     {"TYPE": "hetzner", "ZONES": "example.com", "SCOPE": "private"},
		"bad force":     {"TYPE": "hetzner", "ZONES": "example.com", "FORCE": "yes please"},
		"bad proxied":   {"TYPE": "cloudflare", "ZONES": "example.com", "PROXIED": "maybe"},
		"bad insecure":  {"TYPE": "unifi", "ZONES": "example.com", "INSECURE": "maybe"},
	}
	for name, vars := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := newProvider("X", func(key string) string { return vars[key] })
			assert.Error(t, err)
		})
	}
}

func TestCloudflareProxiedDefault(t *testing.T) {
	t.Parallel()
	proxied := func(scope, value string) bool {
		c, err := newCloudflare(func(key string) string {
			if key == "PROXIED" {
				return value
			}
			return ""
		}, scope)
		require.NoError(t, err)
		return c.(*cloudflare.Provider).HTTPClient != nil
	}

	assert.True(t, proxied("public", ""), "public defaults to proxied")
	assert.False(t, proxied("local", ""), "local must not proxy private IPs by default")
	assert.False(t, proxied("public", "false"))
	assert.True(t, proxied("local", "true"))
}

func TestProxiedClient(t *testing.T) {
	t.Parallel()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = nil
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
	}))
	defer srv.Close()
	c := &proxiedClient{client: srv.Client()}

	send := func(method, body string) {
		req, err := http.NewRequestWithContext(t.Context(), method, srv.URL, strings.NewReader(body))
		require.NoError(t, err)
		resp, err := c.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	send(http.MethodPost, `{"type":"A","name":"app","content":"1.2.3.4"}`)
	assert.Equal(t, true, got["proxied"])
	assert.Equal(t, "1.2.3.4", got["content"])

	send(http.MethodPost, `{"type":"TXT","name":"relayd.app","content":"x"}`)
	assert.NotContains(t, got, "proxied", "TXT records can't be proxied")
}

func TestFromLibDNSNormalizes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   libdns.RR
		want Record
	}{
		{
			libdns.RR{Type: "txt", Name: "Relayd.App", Data: `"managed-by=relayd-x"`},
			Record{Type: "TXT", Name: "relayd.app", Value: "managed-by=relayd-x"},
		},
		{
			libdns.RR{Type: "AAAA", Name: "", Data: "2001:0db8:0:0::1", TTL: time.Minute},
			Record{Type: "AAAA", Name: "@", Value: "2001:db8::1", TTL: time.Minute},
		},
		{libdns.RR{Type: "A", Name: "app", Data: "1.2.3.4"}, Record{Type: "A", Name: "app", Value: "1.2.3.4"}},
	}
	for _, tt := range tests {
		got := fromLibDNS(tt.in)
		got.raw = nil
		assert.Equal(t, tt.want, got)
	}
}

func TestParseTTL(t *testing.T) {
	t.Parallel()
	tests := map[string]time.Duration{
		"":       0,
		"0":      0,
		"300":    5 * time.Minute,
		"5m":     5 * time.Minute,
		"1h30m":  90 * time.Minute,
		"10":     time.Minute,
		"999999": 24 * time.Hour,
		"90.4s":  90 * time.Second,
	}
	for in, want := range tests {
		got, err := ParseTTL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	_, err := ParseTTL("soon")
	assert.Error(t, err)
}

func TestProviderTTLDefaults(t *testing.T) {
	t.Parallel()
	load := func(vars map[string]string) *Provider {
		vars["ZONES"] = "example.com"
		p, err := newProvider("X", func(key string) string { return vars[key] })
		require.NoError(t, err)
		return p
	}

	assert.Zero(t, load(map[string]string{"TYPE": "cloudflare"}).TTL, "cloudflare has auto TTL")
	assert.Zero(t, load(map[string]string{"TYPE": "unifi"}).TTL, "unifi has auto TTL")
	assert.Equal(t, 5*time.Minute, load(map[string]string{"TYPE": "route53"}).TTL, "route53 treats 0 as 0")
	assert.Equal(t, time.Hour, load(map[string]string{"TYPE": "powerdns", "TTL": "1h"}).TTL)

	pi := load(map[string]string{"TYPE": "pihole", "TTL": "1h"})
	assert.Zero(t, pi.TTL)
	assert.True(t, pi.IgnoreTTL)
}

// fakeClient records calls and can not update records. fakeSetter can.
type fakeClient struct {
	calls []string
}

func (f *fakeClient) GetRecords(context.Context, string) ([]libdns.Record, error) { return nil, nil }

func (f *fakeClient) AppendRecords(_ context.Context, _ string, r []libdns.Record) ([]libdns.Record, error) {
	f.calls = append(f.calls, "append "+r[0].RR().Name)
	return r, nil
}

func (f *fakeClient) DeleteRecords(_ context.Context, _ string, r []libdns.Record) ([]libdns.Record, error) {
	f.calls = append(f.calls, "delete "+r[0].RR().Name)
	return r, nil
}

type fakeSetter struct {
	fakeClient

	set []libdns.RR
}

func (f *fakeSetter) SetRecords(_ context.Context, _ string, r []libdns.Record) ([]libdns.Record, error) {
	f.calls = append(f.calls, "set "+r[0].RR().Name)
	f.set = append(f.set, r[0].RR())
	return r, nil
}

func TestApplyOrder(t *testing.T) {
	t.Parallel()
	c := &fakeSetter{}
	p := &Provider{client: c}
	err := p.Apply(t.Context(), "example.com.", ChangeSet{
		Create: []Record{{Type: "A", Name: "new", Value: "1.2.3.4"}},
		Update: []Record{{Type: "A", Name: "ttl", Value: "1.2.3.4", TTL: time.Minute}},
		Delete: []Record{{Type: "A", Name: "old", Value: "1.2.3.4"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"append new", "set ttl", "delete old"}, c.calls)
	assert.Equal(t, time.Minute, c.set[0].TTL, "update must carry the new TTL")
}

func TestApplyUpdateUnsupported(t *testing.T) {
	t.Parallel()
	p := &Provider{client: &fakeClient{}}
	err := p.Apply(t.Context(), "example.com.", ChangeSet{
		Update: []Record{{Type: "A", Name: "ttl", Value: "1.2.3.4", TTL: time.Minute}},
	})
	assert.Error(t, err)
}
