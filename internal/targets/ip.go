// Package targets handles the resolution of target IP addresses for local and public interfaces.
package targets

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mizuchilabs/relayd/internal/util"
	"golang.org/x/sync/errgroup"
)

type IPs struct {
	IPv4 []string
	IPv6 []string
}

func (i IPs) HasAny() bool {
	return len(i.IPv4) > 0 || len(i.IPv6) > 0
}

// virtualPrefixes lists interface name prefixes that belong to virtual/container networks
// and should be excluded from local IP discovery.
var virtualPrefixes = []string{
	"docker", "veth", "br-", "virbr", "cni", "flannel",
	"cali", "tunl", "weave", "lxc", "lxd",
}

func isVirtualInterface(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func ResolveLocalIP() (IPs, error) {
	var ips IPs

	if override := os.Getenv("RELAYD_LOCAL_OVERRIDE_IPV4"); override != "" {
		ips.IPv4 = util.SplitCSV(override)
	}
	if override := os.Getenv("RELAYD_LOCAL_OVERRIDE_IPV6"); override != "" {
		ips.IPv6 = util.SplitCSV(override)
	}
	if len(ips.IPv4) > 0 || len(ips.IPv6) > 0 {
		return ips, nil
	}

	if isInsideContainer() {
		slog.Warn("Running inside a container without local IP overrides. " +
			"Auto-detected IPs may be container-internal and not routable. " +
			"Consider setting RELAYD_LOCAL_OVERRIDE_IPV4/IPV6 or using network_mode: host")
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ips, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	seen4 := make(map[string]struct{})
	seen6 := make(map[string]struct{})

	for _, iface := range ifaces {
		// Skip loopback, down, and virtual interfaces
		if iface.Flags&net.FlagLoopback != 0 ||
			iface.Flags&net.FlagUp == 0 ||
			isVirtualInterface(iface.Name) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			slog.Debug("Failed to get addresses for interface", "iface", iface.Name, "error", err)
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP

			// Skip link-local addresses
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}

			if ip.To4() != nil {
				ipStr := ip.String()
				if _, ok := seen4[ipStr]; !ok {
					seen4[ipStr] = struct{}{}
					ips.IPv4 = append(ips.IPv4, ipStr)
				}
			} else if ip.To16() != nil {
				ipStr := ip.String()
				if _, ok := seen6[ipStr]; !ok {
					seen6[ipStr] = struct{}{}
					ips.IPv6 = append(ips.IPv6, ipStr)
				}
			}
		}
	}

	if !ips.HasAny() {
		return ips, fmt.Errorf("unable to detect any local IP")
	}
	return ips, nil
}

// isInsideContainer checks if the process is running inside a container.
func isInsideContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Fallback: check for container cgroup
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		s := string(data)
		if strings.Contains(s, "docker") || strings.Contains(s, "containerd") ||
			strings.Contains(s, "kubepods") || strings.Contains(s, "/lxc/") {
			return true
		}
	}
	return false
}

var (
	ipv4Providers = []string{
		"https://api.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}
	ipv6Providers = []string{
		"https://api6.ipify.org",
		"https://v6.ident.me",
	}
)

func ResolvePublicIP(ctx context.Context) (IPs, error) {
	var ips IPs

	if override := os.Getenv("RELAYD_PUBLIC_OVERRIDE_IPV4"); override != "" {
		ips.IPv4 = util.SplitCSV(override)
	}
	if override := os.Getenv("RELAYD_PUBLIC_OVERRIDE_IPV6"); override != "" {
		ips.IPv6 = util.SplitCSV(override)
	}

	if len(ips.IPv4) > 0 && len(ips.IPv6) > 0 {
		return ips, nil
	}

	client := &http.Client{Timeout: 5 * time.Second}
	g, gCtx := errgroup.WithContext(ctx)

	if len(ips.IPv4) == 0 {
		g.Go(func() error {
			if ip := fetchIPWithFallback(gCtx, client, ipv4Providers, "IPv4"); ip != "" && strings.Contains(ip, ".") {
				ips.IPv4 = []string{ip}
			}
			return nil
		})
	}

	if len(ips.IPv6) == 0 {
		g.Go(func() error {
			if ip := fetchIPWithFallback(gCtx, client, ipv6Providers, "IPv6"); ip != "" && strings.Contains(ip, ":") {
				ips.IPv6 = []string{ip}
			}
			return nil
		})
	}

	_ = g.Wait()

	if !ips.HasAny() {
		return ips, fmt.Errorf("no public IPs could be resolved")
	}

	return ips, nil
}

// fetchIPWithFallback tries each provider URL in order and returns the first valid IP.
// If all providers fail, it logs a single summary instead of per-provider noise.
func fetchIPWithFallback(ctx context.Context, client *http.Client, providers []string, family string) string {
	for _, url := range providers {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		_ = resp.Body.Close()

		ipStr := strings.TrimSpace(string(body))
		if net.ParseIP(ipStr) != nil {
			return ipStr
		}
	}
	slog.Debug("No public IP resolved, connectivity may not be available", "family", family)
	return ""
}

