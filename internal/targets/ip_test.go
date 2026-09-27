package targets

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResolverFamily(t *testing.T) {
	_, err := NewResolver("IPv4")
	require.Error(t, err)

	r, err := NewResolver("dual")
	require.NoError(t, err)
	assert.True(t, r.want4)
	assert.True(t, r.want6)
}

func TestOverrides(t *testing.T) {
	t.Setenv("RELAYD_LOCAL_OVERRIDE_IPV4", " 192.168.1.10 ")
	t.Setenv("RELAYD_LOCAL_OVERRIDE_IPV6", "fd00::1")
	t.Setenv("RELAYD_PUBLIC_OVERRIDE_IPV6", "2001:db8:0:0::1")

	r, err := NewResolver("ipv4")
	require.NoError(t, err)
	assert.Equal(t, IPs{IPv4: "192.168.1.10"}, r.local, "ipv6 override ignored for ipv4 family")

	ips, err := r.Resolve(t.Context(), "local")
	require.NoError(t, err)
	assert.Equal(t, IPs{IPv4: "192.168.1.10"}, ips)

	r, err = NewResolver("ipv6")
	require.NoError(t, err)
	ips, err = r.Resolve(t.Context(), "public")
	require.NoError(t, err)
	assert.Equal(t, IPs{IPv6: "2001:db8::1"}, ips, "override needs no network and is normalized")
}

func TestOverrideInvalid(t *testing.T) {
	t.Setenv("RELAYD_PUBLIC_OVERRIDE_IPV4", "fd00::1")
	_, err := NewResolver("ipv4")
	assert.ErrorContains(t, err, "RELAYD_PUBLIC_OVERRIDE_IPV4")
}

func TestFetchIPFallsBack(t *testing.T) {
	t.Parallel()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>rate limited</html>"))
	}))
	defer bad.Close()
	v6 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("2001:db8::1\n"))
	}))
	defer v6.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("203.0.113.7\n"))
	}))
	defer good.Close()

	r := &Resolver{http: http.DefaultClient}
	services := []string{bad.URL, v6.URL, good.URL}
	assert.Equal(t, "203.0.113.7", r.fetchIP(t.Context(), services, netip.Addr.Is4))
	assert.Equal(t, "2001:db8::1", r.fetchIP(t.Context(), services, netip.Addr.Is6))
	assert.Empty(t, r.fetchIP(t.Context(), []string{bad.URL}, netip.Addr.Is4))
}
