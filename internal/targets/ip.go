// Package targets finds the IP addresses relayd publishes.
package targets

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	ipv4Services = []string{"https://api.ipify.org", "https://icanhazip.com", "https://ifconfig.me/ip"}
	ipv6Services = []string{"https://api6.ipify.org", "https://v6.ident.me"}

	// Interfaces from container and VM networking are never the host's LAN address.
	virtualPrefixes = []string{
		"docker", "veth", "br-", "virbr", "cni", "flannel",
		"cali", "tunl", "weave", "lxc", "lxd",
	}
)

type IPs struct {
	IPv4 string
	IPv6 string
}

func (i IPs) HasAny() bool {
	return i.IPv4 != "" || i.IPv6 != ""
}

type Resolver struct {
	want4, want6 bool
	local        IPs
	public       IPs
	http         *http.Client
}

// NewResolver validates the IP family and reads the RELAYD_{LOCAL,PUBLIC}_OVERRIDE_IPV{4,6} overrides.
func NewResolver(family string) (*Resolver, error) {
	r := &Resolver{http: &http.Client{Timeout: 5 * time.Second}}
	switch family {
	case "ipv4":
		r.want4 = true
	case "ipv6":
		r.want6 = true
	case "dual":
		r.want4, r.want6 = true, true
	default:
		return nil, fmt.Errorf("ip family must be ipv4, ipv6 or dual, got %q", family)
	}

	var err error
	if r.local, err = r.overrides("LOCAL"); err != nil {
		return nil, err
	}
	if r.public, err = r.overrides("PUBLIC"); err != nil {
		return nil, err
	}
	return r, nil
}

// Resolve returns the addresses to publish for a provider scope ("local" or "public").
func (r *Resolver) Resolve(ctx context.Context, scope string) (IPs, error) {
	var ips IPs
	if scope == "local" {
		ips = r.resolveLocal(ctx)
	} else {
		ips = r.resolvePublic(ctx)
	}
	if !ips.HasAny() {
		return ips, fmt.Errorf("no %s IP found", scope)
	}
	return ips, nil
}

func (r *Resolver) overrides(kind string) (IPs, error) {
	var ips IPs
	var err error
	if r.want4 {
		if ips.IPv4, err = override("RELAYD_"+kind+"_OVERRIDE_IPV4", netip.Addr.Is4); err != nil {
			return ips, err
		}
	}
	if r.want6 {
		if ips.IPv6, err = override("RELAYD_"+kind+"_OVERRIDE_IPV6", netip.Addr.Is6); err != nil {
			return ips, err
		}
	}
	return ips, nil
}

func override(key string, is func(netip.Addr) bool) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", nil
	}
	ip, err := netip.ParseAddr(value)
	if err != nil || !is(ip) {
		return "", fmt.Errorf("%s: invalid address %q", key, value)
	}
	return ip.String(), nil
}

func (r *Resolver) resolveLocal(ctx context.Context) IPs {
	ips := r.local
	if r.want4 && ips.IPv4 == "" {
		ips.IPv4 = outboundIP(ctx, "udp4", "1.1.1.1:53")
	}
	if r.want6 && ips.IPv6 == "" {
		ips.IPv6 = outboundIP(ctx, "udp6", "[2606:4700:4700::1111]:53")
	}
	if (r.want4 && ips.IPv4 == "") || (r.want6 && ips.IPv6 == "") {
		r.fromInterfaces(&ips)
	}
	return ips
}

// outboundIP asks the kernel which source address it would use to reach addr. UDP sends no packets here.
func outboundIP(ctx context.Context, network, addr string) string {
	conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
	if err != nil {
		return ""
	}
	defer func() { _ = conn.Close() }()
	if udp, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return udp.IP.String()
	}
	return ""
}

func (r *Resolver) fromInterfaces(ips *IPs) {
	ifaces, err := net.Interfaces()
	if err != nil {
		slog.Debug("Failed to list network interfaces", "error", err)
		return
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 || isVirtual(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			prefix, err := netip.ParsePrefix(addr.String())
			if err != nil || prefix.Addr().IsLinkLocalUnicast() {
				continue
			}
			ip := prefix.Addr().Unmap()
			if r.want4 && ip.Is4() && ips.IPv4 == "" {
				ips.IPv4 = ip.String()
			}
			if r.want6 && ip.Is6() && ips.IPv6 == "" {
				ips.IPv6 = ip.String()
			}
		}
	}
}

func isVirtual(name string) bool {
	name = strings.ToLower(name)
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (r *Resolver) resolvePublic(ctx context.Context) IPs {
	ips := r.public
	var wg sync.WaitGroup
	if r.want4 && ips.IPv4 == "" {
		wg.Go(func() { ips.IPv4 = r.fetchIP(ctx, ipv4Services, netip.Addr.Is4) })
	}
	if r.want6 && ips.IPv6 == "" {
		wg.Go(func() { ips.IPv6 = r.fetchIP(ctx, ipv6Services, netip.Addr.Is6) })
	}
	wg.Wait()
	return ips
}

// fetchIP returns the first valid address from services, tried in order.
func (r *Resolver) fetchIP(ctx context.Context, services []string, is func(netip.Addr) bool) string {
	for _, url := range services {
		ip, err := r.fetch(ctx, url)
		if err == nil && is(ip) {
			return ip.String()
		}
		slog.Debug("Public IP lookup failed", "url", url, "ip", ip, "error", err)
	}
	return ""
}

func (r *Resolver) fetch(ctx context.Context, url string) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return netip.Addr{}, err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.ParseAddr(strings.TrimSpace(string(body)))
}
