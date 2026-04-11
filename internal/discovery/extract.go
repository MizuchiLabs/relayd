package discovery

import (
	"regexp"
	"strings"

	"github.com/mizuchilabs/relayd/internal/util"
)

var hostRuleRegex = regexp.MustCompile(`Host\(([^)]*)\)`)

// extractHostnames pulls hostnames from container labels.
func extractHostnames(labels map[string]string) []string {
	var hosts []string

	// Manual label
	if val, ok := labels["relayd.hosts"]; ok {
		for v := range strings.SplitSeq(val, ",") {
			if h := util.NormalizeHostname(v); h != "" {
				hosts = append(hosts, h)
			}
		}
	}

	// Traefik Extract
	for key, value := range labels {
		if !strings.HasPrefix(key, "traefik.http.routers.") || !strings.HasSuffix(key, ".rule") {
			continue
		}
		for _, match := range hostRuleRegex.FindAllStringSubmatch(value, -1) {
			if len(match) > 1 {
				hosts = append(hosts, util.ParseQuotedValues(match[1])...)
			}
		}
	}

	return hosts
}
