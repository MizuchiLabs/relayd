// Package targets handles the resolution of target IP addresses for local and public interfaces.
package targets

import (
	"context"
	"fmt"
	"io"
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

func ResolveLocalIP() (IPs, error) {
	var ips IPs

	if override := os.Getenv("RELAYD_LOCAL_OVERRIDE_IPV4"); override != "" {
		ips.IPv4 = util.SplitCSV(override)
	} else if conn, err := net.Dial("udp4", "8.8.8.8:80"); err == nil {
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			ips.IPv4 = []string{addr.IP.String()}
		}
		_ = conn.Close()
	}

	if override := os.Getenv("RELAYD_LOCAL_OVERRIDE_IPV6"); override != "" {
		ips.IPv6 = util.SplitCSV(override)
	} else if conn, err := net.Dial("udp6", "[2001:4860:4860::8888]:80"); err == nil {
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			ips.IPv6 = []string{addr.IP.String()}
		}
		_ = conn.Close()
	}

	if !ips.HasAny() {
		return ips, fmt.Errorf("unable to detect any local IP")
	}
	return ips, nil
}

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
			req, _ := http.NewRequestWithContext(gCtx, http.MethodGet, "https://api.ipify.org", nil)
			resp, err := client.Do(req)
			if err != nil {
				return nil
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
			ipStr := strings.TrimSpace(string(body))
			if net.ParseIP(ipStr) != nil && strings.Contains(ipStr, ".") {
				ips.IPv4 = []string{ipStr}
			}
			return nil
		})
	}

	if len(ips.IPv6) == 0 {
		g.Go(func() error {
			req, _ := http.NewRequestWithContext(
				gCtx,
				http.MethodGet,
				"https://api6.ipify.org",
				nil,
			)
			resp, err := client.Do(req)
			if err != nil {
				return nil
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
			ipStr := strings.TrimSpace(string(body))
			if net.ParseIP(ipStr) != nil && strings.Contains(ipStr, ":") {
				ips.IPv6 = []string{ipStr}
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
