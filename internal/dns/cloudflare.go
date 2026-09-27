package dns

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/libdns/cloudflare"
)

// Proxying a private IP breaks the record, so only public providers proxy by default.
func newCloudflare(env env, scope string) (client, error) {
	proxied, err := parseBool(env("PROXIED"), scope == "public")
	if err != nil {
		return nil, err
	}
	p := &cloudflare.Provider{APIToken: env("TOKEN")}
	if proxied {
		p.HTTPClient = &proxiedClient{client: &http.Client{Timeout: 30 * time.Second}}
	}
	return p, nil
}

// proxiedClient sets "proxied": true on A, AAAA and CNAME writes, since libdns/cloudflare has no option for it.
type proxiedClient struct {
	client *http.Client
}

func (c *proxiedClient) Do(req *http.Request) (*http.Response, error) {
	if req.Body == nil ||
		(req.Method != http.MethodPost && req.Method != http.MethodPut && req.Method != http.MethodPatch) {
		return c.client.Do(req) // #nosec G704 - host validated
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]any
	if json.Unmarshal(body, &data) == nil {
		if t, _ := data["type"].(string); t == "A" || t == "AAAA" || t == "CNAME" {
			data["proxied"] = true
			if patched, err := json.Marshal(data); err == nil {
				body = patched
			}
		}
	}

	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return c.client.Do(req) // #nosec G704 - host validated
}
