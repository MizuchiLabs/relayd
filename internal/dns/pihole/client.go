// Package pihole provides a client for the Pi-hole API.
package pihole

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/libdns/libdns"
)

type Client struct {
	Server   string
	Password string
}

type apiResponse struct {
	Success bool       `json:"success"`
	Message string     `json:"message"`
	Data    [][]string `json:"data"` // For get action
}

func (c *Client) doRequest(ctx context.Context, action string, q url.Values) (*apiResponse, error) {
	reqURL := fmt.Sprintf("%s/admin/api.php", strings.TrimRight(c.Server, "/"))
	if q == nil {
		q = url.Values{}
	}
	q.Set("auth", c.Password)
	q.Set("action", action)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Some endpoints return empty on success, or JSON
	if len(bodyBytes) == 0 {
		return &apiResponse{Success: true}, nil
	}

	var result apiResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		// Pihole might return non-json on error or success sometimes?
		// Usually it's JSON: {"success":true,"message":""}
		return nil, fmt.Errorf(
			"failed to parse pi-hole response: %s (body: %s)",
			err,
			string(bodyBytes),
		)
	}

	if !result.Success && action != "get" && result.Message != "" {
		// Ignore "already exists" errors when adding
		if action == "add" && strings.Contains(strings.ToLower(result.Message), "already exist") {
			return &result, nil
		}
		// Ignore "doesn't exist" when deleting
		if action == "delete" &&
			strings.Contains(strings.ToLower(result.Message), "does not exist") {
			return &result, nil
		}
		return &result, fmt.Errorf("pi-hole api error: %s", result.Message)
	}

	return &result, nil
}

func (c *Client) getRecords(ctx context.Context, endpoint string) ([][]string, error) {
	q := url.Values{}
	q.Set(endpoint, "")
	res, err := c.doRequest(ctx, "get", q)
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

func (c *Client) addRecord(ctx context.Context, rec libdns.RR) error {
	q := url.Values{}
	switch rec.Type {
	case "A", "AAAA":
		q.Set("customdns", "")
		q.Set("domain", rec.Name)
		q.Set("ip", rec.Data)
	case "CNAME":
		q.Set("customcname", "")
		q.Set("domain", rec.Name)
		q.Set("target", rec.Data)
	default:
		return nil
	}
	_, err := c.doRequest(ctx, "add", q)
	return err
}

func (c *Client) deleteRecord(ctx context.Context, rec libdns.RR) error {
	q := url.Values{}

	switch rec.Type {
	case "A", "AAAA":
		q.Set("customdns", "")
		q.Set("domain", rec.Name)
		q.Set("ip", rec.Data)
	case "CNAME":
		q.Set("customcname", "")
		q.Set("domain", rec.Name)
		q.Set("target", rec.Data)
	default:
		return nil
	}

	_, err := c.doRequest(ctx, "delete", q)
	return err
}
