package discovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func names(hosts []Host) []string {
	var out []string
	for _, h := range hosts {
		out = append(out, h.Name)
	}
	return out
}

func TestHostsFromLabelsNeedsEnable(t *testing.T) {
	t.Parallel()
	assert.Empty(t, hostsFromLabels(nil))
	assert.Empty(t, hostsFromLabels(map[string]string{"relayd.hosts": "app.example.com"}))
	assert.Empty(t, hostsFromLabels(map[string]string{"relayd.enable": "false", "relayd.hosts": "app.example.com"}))
}

func TestHostsFromLabelsManual(t *testing.T) {
	t.Parallel()
	hosts := hostsFromLabels(map[string]string{
		"relayd.enable": "true",
		"relayd.hosts":  " App.Example.com. ,, b.example.com,bad host",
	})
	assert.Equal(t, []string{"app.example.com", "b.example.com"}, names(hosts))
}

func TestHostsFromLabelsTraefik(t *testing.T) {
	t.Parallel()
	hosts := hostsFromLabels(map[string]string{
		"relayd.enable":                      "true",
		"traefik.http.routers.a.rule":        "Host(`a.example.com`) && PathPrefix(`/api`)",
		"traefik.http.routers.b.rule":        `Host("b.example.com") || Host('c.example.com')`,
		"traefik.http.routers.c.rule":        "Host(`d.example.com`, `e.example.com`)",
		"traefik.http.routers.d.rule":        "HostRegexp(`{sub:[a-z]+}.example.com`)",
		"traefik.tcp.routers.e.rule":         "HostSNI(`tcp.example.com`)",
		"traefik.http.routers.a.entrypoints": "websecure",
	})
	assert.ElementsMatch(t, []string{
		"a.example.com", "b.example.com", "c.example.com", "d.example.com", "e.example.com",
	}, names(hosts))
}

func TestHostsFromLabelsProviders(t *testing.T) {
	t.Parallel()
	hosts := hostsFromLabels(map[string]string{
		"relayd.enable":    "true",
		"relayd.hosts":     "app.example.com",
		"relayd.providers": "cloudflare, local,",
	})
	assert.Equal(t, []Host{{Name: "app.example.com", Providers: []string{"cloudflare", "local"}}}, hosts)
}

func TestHostsFromLabelsTTL(t *testing.T) {
	t.Parallel()
	hosts := hostsFromLabels(map[string]string{
		"relayd.enable": "true",
		"relayd.hosts":  "app.example.com",
		"relayd.ttl":    "5m",
	})
	assert.Equal(t, 5*time.Minute, hosts[0].TTL)

	hosts = hostsFromLabels(map[string]string{
		"relayd.enable": "true",
		"relayd.hosts":  "app.example.com",
		"relayd.ttl":    "soon",
	})
	assert.Zero(t, hosts[0].TTL, "invalid TTL falls back to the provider default")
}
