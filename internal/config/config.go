// Package config handles the parsing and structuring of configuration options for relayd.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/mizuchilabs/relayd/internal/util"
	"github.com/urfave/cli/v3"
)

// Config holds the configuration for relayd.
type Config struct {
	InstanceID   string
	Force        bool
	SyncInterval time.Duration
	Providers    []Provider
}

// Provider holds the configuration for a DNS provider.
type Provider struct {
	Type  string
	Scope string
	Zones []string
	Token string
}

// New creates a new Config from a CLI command.
func New(cmd *cli.Command) Config {
	cfg := Config{
		InstanceID:   generateInstanceID(),
		Force:        cmd.Bool("force"),
		SyncInterval: cmd.Duration("sync-interval"),
	}

	for _, name := range util.SplitCSV(cmd.String("providers")) {
		name = strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
		pfx := "RELAYD_PROVIDER_" + name + "_"
		cfg.Providers = append(cfg.Providers, Provider{
			Type:  os.Getenv(pfx + "TYPE"),
			Scope: util.GetEnv(pfx+"SCOPE", "public"),
			Zones: util.SplitCSV(os.Getenv(pfx + "ZONES")),
			Token: os.Getenv(pfx + "TOKEN"),
		})
	}

	return cfg
}

func generateInstanceID() string {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b))
		}
	}
	h, _ := os.Hostname()
	if h == "" {
		h = "relayd"
	}
	hash := sha256.Sum256([]byte(h))
	return hex.EncodeToString(hash[:])[:12]
}
