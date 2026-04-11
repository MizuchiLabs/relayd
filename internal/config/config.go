// Package config handles the parsing and structuring of configuration options for relayd.
package config

import (
	"os"
	"strings"
	"time"

	"github.com/mizuchilabs/relayd/internal/util"
	"github.com/urfave/cli/v3"
)

// Config holds the configuration for relayd.
type Config struct {
	Force     bool
	Interval  time.Duration
	Providers []Provider
}

// Provider holds the configuration for a DNS provider.
type Provider struct {
	Name  string
	Type  string
	Scope string
	Zones []string
	Token string
	URL   string
}

func New(cmd *cli.Command) Config {
	cfg := Config{
		Force:    cmd.Bool("force"),
		Interval: cmd.Duration("interval"),
	}

	// Auto-discover providers by scanning environment variables
	providerNames := make(map[string]struct{})
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		key := parts[0]
		if strings.HasPrefix(key, "RELAYD_PROVIDER_") && strings.HasSuffix(key, "_TYPE") {
			// Extract the <NAME> part from RELAYD_PROVIDER_<NAME>_TYPE
			name := strings.TrimSuffix(strings.TrimPrefix(key, "RELAYD_PROVIDER_"), "_TYPE")
			if name != "" {
				providerNames[name] = struct{}{}
			}
		}
	}

	for name := range providerNames {
		pfx := "RELAYD_PROVIDER_" + name + "_"
		cfg.Providers = append(cfg.Providers, Provider{
			Name:  name,
			Type:  os.Getenv(pfx + "TYPE"),
			Scope: util.GetEnv(pfx+"SCOPE", "public"),
			Zones: util.SplitCSV(os.Getenv(pfx + "ZONES")),
			Token: os.Getenv(pfx + "TOKEN"),
			URL:   os.Getenv(pfx + "URL"),
		})
	}

	return cfg
}
