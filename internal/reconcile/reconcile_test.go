package reconcile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mizuchilabs/relayd/internal/dns"
	"github.com/mizuchilabs/relayd/internal/targets"
)

const owner = "managed-by=relayd-test"

func desired(hosts ...string) Desired {
	d := Desired{
		Zone:     "example.com",
		Instance: "test",
		Hosts:    map[string]time.Duration{},
		IPs:      targets.IPs{IPv4: "1.2.3.4"},
	}
	for _, h := range hosts {
		d.Hosts[h] = 0
	}
	return d
}

func rec(typ, name, value string) dns.Record {
	return dns.Record{Type: typ, Name: name, Value: value}
}

func TestPlanCreatesTXTBeforeA(t *testing.T) {
	t.Parallel()
	changes := Plan(desired("app.example.com"), nil)

	assert.Equal(t, []dns.Record{
		rec("TXT", "relayd.app", owner),
		rec("A", "app", "1.2.3.4"),
	}, changes.Create)
	assert.Empty(t, changes.Delete)
}

func TestPlanApex(t *testing.T) {
	t.Parallel()
	changes := Plan(desired("example.com"), nil)

	assert.Equal(t, []dns.Record{
		rec("TXT", "relayd", owner),
		rec("A", "@", "1.2.3.4"),
	}, changes.Create)
}

func TestPlanDualStack(t *testing.T) {
	t.Parallel()
	d := desired("app.example.com")
	d.IPs.IPv6 = "2001:db8::1"
	changes := Plan(d, nil)

	assert.Equal(t, []dns.Record{
		rec("TXT", "relayd.app", owner),
		rec("A", "app", "1.2.3.4"),
		rec("AAAA", "app", "2001:db8::1"),
	}, changes.Create)
}

func TestPlanNoChangesWhenInSync(t *testing.T) {
	t.Parallel()
	changes := Plan(desired("app.example.com"), []dns.Record{
		rec("TXT", "relayd.app", owner),
		rec("A", "app", "1.2.3.4"),
	})
	assert.True(t, changes.Empty(), "in-sync zone should produce no changes, got %+v", changes)
}

func TestPlanIgnoresHostsOutsideZone(t *testing.T) {
	t.Parallel()
	changes := Plan(desired("app.other.com", "app.notexample.com"), nil)
	assert.True(t, changes.Empty())
}

func TestPlanCreatesWithTTL(t *testing.T) {
	t.Parallel()
	d := desired()
	d.Hosts["app.example.com"] = 5 * time.Minute
	changes := Plan(d, nil)

	assert.Equal(t, []dns.Record{
		{Type: "TXT", Name: "relayd.app", Value: owner, TTL: 5 * time.Minute},
		{Type: "A", Name: "app", Value: "1.2.3.4", TTL: 5 * time.Minute},
	}, changes.Create)
}

func TestPlanUpdatesChangedTTL(t *testing.T) {
	t.Parallel()
	d := desired()
	d.Hosts["app.example.com"] = 5 * time.Minute
	changes := Plan(d, []dns.Record{
		{Type: "TXT", Name: "relayd.app", Value: owner},
		{Type: "A", Name: "app", Value: "1.2.3.4", TTL: time.Hour},
	})

	assert.Empty(t, changes.Create)
	assert.Empty(t, changes.Delete)
	assert.Equal(t, []dns.Record{{Type: "A", Name: "app", Value: "1.2.3.4", TTL: 5 * time.Minute}}, changes.Update,
		"TXT must not be updated, UniFi keeps no TTL on it")
}

func TestPlanAutoTTLIsNotCompared(t *testing.T) {
	t.Parallel()
	changes := Plan(desired("app.example.com"), []dns.Record{
		{Type: "TXT", Name: "relayd.app", Value: owner},
		{Type: "A", Name: "app", Value: "1.2.3.4", TTL: time.Second}, // Cloudflare reports auto as 1s
	})
	assert.True(t, changes.Empty(), "got %+v", changes)
}

func TestPlanSkipsUnownedHost(t *testing.T) {
	t.Parallel()
	changes := Plan(desired("app.example.com"), []dns.Record{
		rec("A", "app", "9.9.9.9"),
	})
	assert.True(t, changes.Empty(), "must not touch a host without our TXT record")
}

func TestPlanSkipsHostOwnedByOtherInstance(t *testing.T) {
	t.Parallel()
	changes := Plan(desired("app.example.com"), []dns.Record{
		rec("TXT", "relayd.app", "managed-by=relayd-other"),
		rec("A", "app", "9.9.9.9"),
	})
	assert.True(t, changes.Empty(), "must not touch a host owned by another instance")
}

func TestPlanIPChange(t *testing.T) {
	t.Parallel()
	changes := Plan(desired("app.example.com"), []dns.Record{
		rec("TXT", "relayd.app", owner),
		rec("A", "app", "9.9.9.9"),
	})

	assert.Equal(t, []dns.Record{rec("A", "app", "1.2.3.4")}, changes.Create)
	assert.Equal(t, []dns.Record{rec("A", "app", "9.9.9.9")}, changes.Delete)
}

func TestPlanRemovesOwnedHostThatWentAway(t *testing.T) {
	t.Parallel()
	changes := Plan(desired(), []dns.Record{
		rec("TXT", "relayd.app", owner),
		rec("A", "app", "1.2.3.4"),
		rec("AAAA", "app", "2001:db8::1"),
	})

	assert.Empty(t, changes.Create)
	assert.Equal(t, []dns.Record{
		rec("A", "app", "1.2.3.4"),
		rec("AAAA", "app", "2001:db8::1"),
		rec("TXT", "relayd.app", owner),
	}, changes.Delete, "address records must be deleted before the TXT")
}

func TestPlanLeavesForeignRecordsAlone(t *testing.T) {
	t.Parallel()
	changes := Plan(desired(), []dns.Record{
		rec("A", "mail", "5.5.5.5"),
		rec("TXT", "@", "v=spf1 -all"),
		rec("TXT", "relayd.other", "managed-by=relayd-other"),
		rec("CNAME", "www", "example.com."),
	})
	assert.True(t, changes.Empty(), "got %+v", changes)
}

func TestPlanSkipsHostWithCNAME(t *testing.T) {
	t.Parallel()
	for _, force := range []bool{false, true} {
		d := desired("www.example.com")
		d.Force = force
		changes := Plan(d, []dns.Record{rec("CNAME", "www", "example.com.")})
		assert.True(t, changes.Empty(), "force=%v: A next to a CNAME is invalid", force)
	}
}

func TestPlanForceCreatesWithoutTXT(t *testing.T) {
	t.Parallel()
	d := desired("app.example.com")
	d.Force = true
	changes := Plan(d, []dns.Record{rec("A", "app", "9.9.9.9")})

	assert.Equal(t, []dns.Record{rec("A", "app", "1.2.3.4")}, changes.Create)
	assert.Equal(t, []dns.Record{rec("A", "app", "9.9.9.9")}, changes.Delete,
		"force mode must replace stale IPs of desired hosts")
}

func TestPlanForceOwnsAllAddressRecords(t *testing.T) {
	t.Parallel()
	d := desired()
	d.Force = true
	changes := Plan(d, []dns.Record{
		rec("A", "mail", "5.5.5.5"),
		rec("CNAME", "www", "example.com."),
		rec("TXT", "@", "v=spf1 -all"),
	})
	assert.Equal(t, []dns.Record{rec("A", "mail", "5.5.5.5")}, changes.Delete)
}

func TestPlanForceCleansUpOwnTXT(t *testing.T) {
	t.Parallel()
	d := desired("app.example.com")
	d.Force = true
	changes := Plan(d, []dns.Record{
		rec("TXT", "relayd.app", owner),
		rec("TXT", "relayd.other", "managed-by=relayd-other"),
		rec("A", "app", "1.2.3.4"),
	})

	assert.Empty(t, changes.Create)
	assert.Equal(t, []dns.Record{rec("TXT", "relayd.app", owner)}, changes.Delete)
}

func TestPlanZoneWithTrailingDotAndCase(t *testing.T) {
	t.Parallel()
	d := desired("app.example.com")
	d.Zone = "Example.COM."
	changes := Plan(d, []dns.Record{
		rec("TXT", "relayd.app", owner),
		rec("A", "app", "1.2.3.4"),
	})
	assert.True(t, changes.Empty(), "got %+v", changes)
}
