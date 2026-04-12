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
	IPv4 string
	IPv6 string
}

func (i IPs) HasAny() bool {
	return i.IPv4 != "" || i.IPv6 != ""
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

func getPreferredIP(network, address string) string {
	// A dummy connection to find the preferred outbound IP.
	// It doesn't actually send packets if it's UDP.
	conn, err := net.Dial(network, address)
	if err != nil {
		return ""
	}
	defer func() { _ = conn.Close() }()

	if udpAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return udpAddr.IP.String()
	}
	return ""
}

func ResolveLocalIP(family string) (IPs, error) {
	var ips IPs

	if family == "ipv4" || family == "dual" || family == "" {
		if override := os.Getenv("RELAYD_LOCAL_OVERRIDE_IPV4"); override != "" {
			if parts := util.SplitCSV(override); len(parts) > 0 {
				ips.IPv4 = parts[0]
			}
		}
	}
	if family == "ipv6" || family == "dual" {
		if override := os.Getenv("RELAYD_LOCAL_OVERRIDE_IPV6"); override != "" {
			if parts := util.SplitCSV(override); len(parts) > 0 {
				ips.IPv6 = parts[0]
			}
		}
	}

	if (family == "ipv4" && ips.IPv4 != "") ||
		(family == "ipv6" && ips.IPv6 != "") ||
		((family == "dual" || family == "") && ips.IPv4 != "" && ips.IPv6 != "") {
		return ips, nil
	}

	if (family == "ipv4" || family == "dual" || family == "") && ips.IPv4 == "" {
		if ip := getPreferredIP("udp4", "1.1.1.1:53"); ip != "" {
			ips.IPv4 = ip
		}
	}
	if (family == "ipv6" || family == "dual") && ips.IPv6 == "" {
		if ip := getPreferredIP("udp6", "[2606:4700:4700::1111]:53"); ip != "" {
			ips.IPv6 = ip
		}
	}

	if (family == "ipv4" && ips.IPv4 != "") ||
		(family == "ipv6" && ips.IPv6 != "") ||
		((family == "dual" || family == "") && ips.IPv4 != "" && ips.IPv6 != "") {
		return ips, nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ips, fmt.Errorf("failed to list network interfaces: %w", err)
	}

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
				if (family == "ipv4" || family == "dual" || family == "") && ips.IPv4 == "" {
					ips.IPv4 = ip.String()
				}
			} else if ip.To16() != nil {
				if (family == "ipv6" || family == "dual") && ips.IPv6 == "" {
					ips.IPv6 = ip.String()
				}
			}

			if (family == "ipv4" && ips.IPv4 != "") ||
				(family == "ipv6" && ips.IPv6 != "") ||
				((family == "dual" || family == "") && ips.IPv4 != "" && ips.IPv6 != "") {
				break
			}
		}

		if (family == "ipv4" && ips.IPv4 != "") ||
			(family == "ipv6" && ips.IPv6 != "") ||
			((family == "dual" || family == "") && ips.IPv4 != "" && ips.IPv6 != "") {
			break
		}
	}

	if !ips.HasAny() {
		return ips, fmt.Errorf("unable to detect any local IP")
	}
	return ips, nil
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

func ResolvePublicIP(ctx context.Context, family string) (IPs, error) {
	var ips IPs

	if family == "ipv4" || family == "dual" || family == "" {
		if override := os.Getenv("RELAYD_PUBLIC_OVERRIDE_IPV4"); override != "" {
			if parts := util.SplitCSV(override); len(parts) > 0 {
				ips.IPv4 = parts[0]
			}
		}
	}
	if family == "ipv6" || family == "dual" {
		if override := os.Getenv("RELAYD_PUBLIC_OVERRIDE_IPV6"); override != "" {
			if parts := util.SplitCSV(override); len(parts) > 0 {
				ips.IPv6 = parts[0]
			}
		}
	}

	if (family == "ipv4" && ips.IPv4 != "") ||
		(family == "ipv6" && ips.IPv6 != "") ||
		((family == "dual" || family == "") && ips.IPv4 != "" && ips.IPv6 != "") {
		return ips, nil
	}

	client := &http.Client{Timeout: 5 * time.Second}
	g, gCtx := errgroup.WithContext(ctx)

	if (family == "ipv4" || family == "dual" || family == "") && ips.IPv4 == "" {
		g.Go(func() error {
			if ip := fetchIPWithFallback(gCtx, client, ipv4Providers, "IPv4"); ip != "" &&
				strings.Contains(ip, ".") {
				ips.IPv4 = ip
			}
			return nil
		})
	}

	if (family == "ipv6" || family == "dual") && ips.IPv6 == "" {
		g.Go(func() error {
			if ip := fetchIPWithFallback(gCtx, client, ipv6Providers, "IPv6"); ip != "" &&
				strings.Contains(ip, ":") {
				ips.IPv6 = ip
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
func fetchIPWithFallback(
	ctx context.Context,
	client *http.Client,
	providers []string,
	family string,
) string {
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
