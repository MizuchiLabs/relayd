// Package pihole is a libdns provider for the Pi-hole v6 API.
package pihole

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Provider struct {
	url      string
	password string
	http     *http.Client

	mu  sync.Mutex
	sid string
}

// New creates a provider for the Pi-hole at url. The password can be the admin or an app password.
func New(url, password string, insecure bool) *Provider {
	client := &http.Client{Timeout: 10 * time.Second}
	if insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 - opt-in for self-signed certs
		}
	}
	return &Provider{
		url:      strings.TrimRight(url, "/") + "/api",
		password: password,
		http:     client,
	}
}

// call sends an API request and logs in again once if the session has expired.
func (p *Provider) call(ctx context.Context, method, path string, out any) error {
	for attempt := range 2 {
		sid, err := p.session(ctx, attempt > 0)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, method, p.url+path, nil)
		if err != nil {
			return err
		}
		if sid != "" {
			req.Header.Set("X-Ftl-Sid", sid)
		}

		status, body, err := p.do(req)
		if err != nil {
			return err
		}
		switch {
		case status == http.StatusUnauthorized && attempt == 0:
			continue
		case status == http.StatusNotFound && method == http.MethodDelete:
			return nil
		case status >= 300:
			return fmt.Errorf("pi-hole %s %s: %d %s", method, path, status, body)
		case out != nil:
			return json.Unmarshal(body, out)
		}
		return nil
	}
	return errors.New("pi-hole: unauthorized")
}

// session returns the current session id, logging in when there is none or renew is set.
// An empty password means the Pi-hole has no auth.
func (p *Provider) session(ctx context.Context, renew bool) (string, error) {
	if p.password == "" {
		return "", nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sid != "" && !renew {
		return p.sid, nil
	}

	payload, err := json.Marshal(map[string]string{"password": p.password})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/auth", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	status, body, err := p.do(req)
	if err != nil {
		return "", err
	}
	var res struct {
		Session struct {
			Valid   bool   `json:"valid"`
			SID     string `json:"sid"`
			Message string `json:"message"`
		} `json:"session"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", fmt.Errorf("pi-hole login: %d %s", status, body)
	}
	if !res.Session.Valid {
		return "", fmt.Errorf("pi-hole login: %s", res.Session.Message)
	}
	p.sid = res.Session.SID
	return p.sid, nil
}

func (p *Provider) do(req *http.Request) (int, []byte, error) {
	resp, err := p.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}
