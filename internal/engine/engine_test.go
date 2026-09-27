package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mizuchilabs/relayd/internal/discovery"
	"github.com/mizuchilabs/relayd/internal/dns"
)

func TestHostsFor(t *testing.T) {
	t.Parallel()
	hosts := []discovery.Host{
		{Name: "all.example.com"},
		{Name: "star.example.com", Providers: []string{"*"}},
		{Name: "byname.example.com", Providers: []string{"cloudflare"}},
		{Name: "byscope.example.com", Providers: []string{"public"}},
		{Name: "local.example.com", Providers: []string{"local"}},
		{Name: "other.example.com", Providers: []string{"pihole"}},
	}
	p := &dns.Provider{Name: "CLOUDFLARE", Scope: "public"}

	assert.Equal(t, map[string]time.Duration{
		"all.example.com": 0, "star.example.com": 0, "byname.example.com": 0, "byscope.example.com": 0,
	}, hostsFor(p, hosts), "label names are matched case-insensitively against the env name")
}

func TestHostsForTTL(t *testing.T) {
	t.Parallel()
	hosts := []discovery.Host{
		{Name: "default.example.com"},
		{Name: "label.example.com", TTL: time.Minute},
		{Name: "shared.example.com", TTL: time.Hour},
		{Name: "shared.example.com"},
		{Name: "shared.example.com", TTL: 2 * time.Minute},
	}

	p := &dns.Provider{Scope: "public", TTL: 10 * time.Minute}
	assert.Equal(t, map[string]time.Duration{
		"default.example.com": 10 * time.Minute,
		"label.example.com":   time.Minute,
		"shared.example.com":  2 * time.Minute,
	}, hostsFor(p, hosts), "label beats provider default, lowest TTL wins")

	auto := &dns.Provider{Scope: "public"}
	assert.Equal(t, 2*time.Minute, hostsFor(auto, hosts)["shared.example.com"], "explicit TTL beats auto")

	pihole := &dns.Provider{Scope: "public", IgnoreTTL: true}
	assert.Zero(t, hostsFor(pihole, hosts)["label.example.com"])
}
