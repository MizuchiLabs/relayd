package dns

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"

	libdnscloudflare "github.com/libdns/cloudflare"
	"github.com/mizuchilabs/relayd/internal/config"
)

type proxiedClient struct {
	client *http.Client
}

func (c *proxiedClient) Do(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodPost || req.Method == http.MethodPut ||
		req.Method == http.MethodPatch {
		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			if err == nil {
				var data map[string]any
				if err := json.Unmarshal(bodyBytes, &data); err == nil {
					// Cloudflare API allows proxying for A, AAAA, and CNAME
					if t, ok := data["type"].(string); ok &&
						(t == "A" || t == "AAAA" || t == "CNAME") {
						data["proxied"] = true
						if newBody, err := json.Marshal(data); err == nil {
							req.Body = io.NopCloser(bytes.NewReader(newBody))
							req.ContentLength = int64(len(newBody))
						} else {
							req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
						}
					} else {
						req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
					}
				} else {
					req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}
		}
	}
	return c.client.Do(req)
}

// NewCloudflareProvider creates a new Cloudflare DNS provider wrapped for relayd.
func NewCloudflareProvider(cfg config.Provider) Provider {
	provider := &libdnscloudflare.Provider{
		APIToken: cfg.Token,
	}

	proxied := os.Getenv("RELAYD_PROVIDER_" + cfg.Name + "_PROXIED")
	if proxied != "false" {
		provider.HTTPClient = &proxiedClient{
			client: http.DefaultClient,
		}
	}

	return &wrapper{
		scope:  cfg.Scope,
		zones:  append([]string(nil), cfg.Zones...),
		client: provider,
	}
}
