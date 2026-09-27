package pihole

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/libdns/libdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePihole mimics the parts of the Pi-hole v6 API relayd uses.
type fakePihole struct {
	mu     sync.Mutex
	hosts  []string
	cnames []string
	sid    string
	logins int
}

func (f *fakePihole) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
		}
		_ = json.UnmarshalRead(r.Body, &body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if body.Password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"session":{"valid":false,"sid":null,"message":"password incorrect"}}`))
			return
		}
		f.logins++
		f.sid = "sid-" + strconv.Itoa(f.logins)
		_ = json.MarshalWrite(w, map[string]any{"session": map[string]any{"valid": true, "sid": f.sid}})
	})

	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			ok := r.Header.Get("X-Ftl-Sid") == f.sid && f.sid != ""
			f.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("GET /api/config/dns/{key}", authed(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.MarshalWrite(w, map[string]any{"config": map[string]any{"dns": map[string]any{
			"hosts":        f.hosts,
			"cnameRecords": f.cnames,
		}}})
	}))
	mux.HandleFunc("PUT /api/config/dns/hosts/{entry}", authed(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hosts = append(f.hosts, r.PathValue("entry"))
		w.WriteHeader(http.StatusCreated)
	}))
	mux.HandleFunc("DELETE /api/config/dns/hosts/{entry}", authed(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		i := slices.Index(f.hosts, r.PathValue("entry"))
		if i < 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.hosts = slices.Delete(f.hosts, i, i+1)
		w.WriteHeader(http.StatusNoContent)
	}))
	return mux
}

func newTestProvider(t *testing.T, f *fakePihole, password string) *Provider {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	return New(srv.URL+"/", password, false)
}

func TestGetRecords(t *testing.T) {
	t.Parallel()
	f := &fakePihole{
		hosts: []string{
			"10.0.0.1 app.home.lan",
			"fd00::1 app.home.lan",
			"10.0.0.2 a.home.lan b.home.lan",
			"10.0.0.3 nothome.lan",
			"10.0.0.4 home.lan",
		},
		cnames: []string{"www.home.lan,app.home.lan,300", "x.other.lan,app.home.lan"},
	}
	p := newTestProvider(t, f, "secret")

	records, err := p.GetRecords(t.Context(), "home.lan")
	require.NoError(t, err)
	assert.Equal(t, []libdns.Record{
		libdns.RR{Type: "A", Name: "app", Data: "10.0.0.1"},
		libdns.RR{Type: "AAAA", Name: "app", Data: "fd00::1"},
		libdns.RR{Type: "A", Name: "a", Data: "10.0.0.2"},
		libdns.RR{Type: "A", Name: "b", Data: "10.0.0.2"},
		libdns.RR{Type: "A", Name: "@", Data: "10.0.0.4"},
		libdns.RR{Type: "CNAME", Name: "www", Data: "app.home.lan"},
	}, records)
}

func TestAppendAndDelete(t *testing.T) {
	t.Parallel()
	f := &fakePihole{}
	p := newTestProvider(t, f, "secret")
	rec := libdns.RR{Type: "A", Name: "app", Data: "10.0.0.1"}

	_, err := p.AppendRecords(t.Context(), "home.lan.", []libdns.Record{rec})
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1 app.home.lan"}, f.hosts)

	_, err = p.DeleteRecords(t.Context(), "home.lan.", []libdns.Record{rec})
	require.NoError(t, err)
	assert.Empty(t, f.hosts)

	_, err = p.DeleteRecords(t.Context(), "home.lan.", []libdns.Record{rec})
	require.NoError(t, err, "deleting a missing entry is not an error")
}

func TestUnsupportedType(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, &fakePihole{}, "secret")
	_, err := p.AppendRecords(t.Context(), "home.lan", []libdns.Record{libdns.RR{Type: "TXT", Name: "x", Data: "y"}})
	assert.Error(t, err)
}

func TestSessionReusedAndRenewed(t *testing.T) {
	t.Parallel()
	f := &fakePihole{}
	p := newTestProvider(t, f, "secret")

	_, err := p.GetRecords(t.Context(), "home.lan")
	require.NoError(t, err)
	_, err = p.GetRecords(t.Context(), "home.lan")
	require.NoError(t, err)
	assert.Equal(t, 1, f.logins, "session should be reused")

	f.mu.Lock()
	f.sid = "expired"
	f.mu.Unlock()
	_, err = p.GetRecords(t.Context(), "home.lan")
	require.NoError(t, err)
	assert.Equal(t, 2, f.logins, "expired session should trigger one new login")
}

func TestWrongPassword(t *testing.T) {
	t.Parallel()
	p := newTestProvider(t, &fakePihole{}, "wrong")
	_, err := p.GetRecords(t.Context(), "home.lan")
	assert.ErrorContains(t, err, "password incorrect")
}
