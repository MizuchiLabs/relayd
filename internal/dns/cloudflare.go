package dns

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	libdnscloudflare "github.com/libdns/cloudflare"
	"github.com/mizuchilabs/relayd/internal/config"
)

type proxiedClient struct {
	client *http.Client
}

func (c *proxiedClient) Do(req *http.Request) (*http.Response, error) {
	// Skip if no body or wrong method
	if req.Body == nil ||
		(req.Method != http.MethodPost && req.Method != http.MethodPut && req.Method != http.MethodPatch) {
		return c.client.Do(req) // #nosec G704 - host validated
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return c.client.Do(req) // #nosec G704 - host validated
	}

	var data map[string]any
	if err := json.Unmarshal(bodyBytes, &data); err == nil {
		if t, ok := data["type"].(string); ok && (t == "A" || t == "AAAA" || t == "CNAME") {
			data["proxied"] = true

			if newBody, err := json.Marshal(data); err == nil {
				req.Body = io.NopCloser(bytes.NewReader(newBody))
				req.ContentLength = int64(len(newBody))
				return c.client.Do(req) // #nosec G704 - host validated
			}
		}
	}

	// Fallback: restore original body if parsing or marshaling failed
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return c.client.Do(req) // #nosec G704 - host validated
}

// NewCloudflareProvider creates a new Cloudflare DNS provider wrapped for relayd.
func NewCloudflareProvider(cfg config.Provider) Provider {
	provider := &libdnscloudflare.Provider{
		APIToken: cfg.Token,
	}

	proxied := os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_PROXIED")
	if proxied != "false" {
		provider.HTTPClient = &proxiedClient{
			client: &http.Client{
				Timeout: 30 * time.Second,
			},
		}
	}

	return newWrapper(cfg, provider)
}
