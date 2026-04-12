// Package unifi provides a client for the UniFi API.
package unifi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL        string
	Token          string
	Site           string
	resolvedSiteID string
	client         *http.Client
}

type DNSPolicy struct {
	ID               string `json:"id,omitempty"`
	Type             string `json:"type,omitempty"`
	Domain           string `json:"domain"`
	Enabled          bool   `json:"enabled"`
	TTLSeconds       int32  `json:"ttlSeconds"`
	IPv4Address      string `json:"ipv4Address,omitempty"`
	IPv6Address      string `json:"ipv6Address,omitempty"`
	TargetDomain     string `json:"targetDomain,omitempty"`
	Text             string `json:"text,omitempty"`
	MailServerDomain string `json:"mailServerDomain,omitempty"`
	Priority         uint16 `json:"priority,omitempty"`
	ServerDomain     string `json:"serverDomain,omitempty"`
	Service          string `json:"service,omitempty"`
	Protocol         string `json:"protocol,omitempty"`
	Port             uint16 `json:"port,omitempty"`
	Weight           uint16 `json:"weight,omitempty"`
}

type listResponse struct {
	Data       []DNSPolicy `json:"data"`
	TotalCount int         `json:"totalCount"`
}

type sitesResponse struct {
	Data []struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		InternalReference string `json:"internalReference"`
	} `json:"data"`
}

func (c *Client) getClient() *http.Client {
	if c.client == nil {
		customTransport := http.DefaultTransport.(*http.Transport).Clone()
		// #nosec G402 - Unifi ships with self-signed certs
		customTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

		c.client = &http.Client{
			Timeout:   10 * time.Second,
			Transport: customTransport,
		}
	}
	return c.client
}

func (c *Client) doRequest(
	ctx context.Context,
	method, url string,
	body io.Reader,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("X-API-KEY", c.Token)
	}

	resp, err := c.getClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(b))
	}
	return b, nil
}

func (c *Client) resolveSiteID(ctx context.Context) (string, error) {
	if c.resolvedSiteID != "" {
		return c.resolvedSiteID, nil
	}
	if c.Site == "" {
		c.Site = "default"
	}

	url := fmt.Sprintf("%s/sites", c.BaseURL)
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to fetch sites: %w", err)
	}

	var sr sitesResponse
	if err := json.Unmarshal(resp, &sr); err != nil {
		return "", fmt.Errorf("failed to unmarshal sites: %w", err)
	}

	for _, site := range sr.Data {
		if site.Name == c.Site || site.ID == c.Site || site.InternalReference == c.Site {
			c.resolvedSiteID = site.ID
			return c.resolvedSiteID, nil
		}
	}
	return "", fmt.Errorf("site '%s' not found", c.Site)
}

func (c *Client) getRecords(ctx context.Context, zone string) ([]DNSPolicy, error) {
	siteID, err := c.resolveSiteID(ctx)
	if err != nil {
		return nil, err
	}
	if siteID == "" {
		return nil, errors.New("site ID not found")
	}

	var all []DNSPolicy
	offset := 0
	limit := 100

	zoneFilter := strings.TrimSuffix(zone, ".")
	filter := fmt.Sprintf("or(domain.eq('%s'),domain.like('*%s'))", zoneFilter, zoneFilter)
	filterEscaped := strings.ReplaceAll(
		filter,
		" ",
		"%20",
	) // simple escape is often enough, or net/url

	for {
		urlStr := fmt.Sprintf(
			"%s/sites/%s/dns/policies?offset=%d&limit=%d&filter=%s",
			c.BaseURL,
			siteID,
			offset,
			limit,
			filterEscaped,
		)
		resp, err := c.doRequest(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list policies: %w", err)
		}

		var list listResponse
		if err := json.Unmarshal(resp, &list); err != nil {
			return nil, fmt.Errorf("failed to unmarshal policies: %w", err)
		}

		all = append(all, list.Data...)
		if len(all) >= list.TotalCount || len(list.Data) == 0 {
			break
		}
		offset += limit
	}

	return all, nil
}

func (c *Client) createRecord(ctx context.Context, policy DNSPolicy) (DNSPolicy, error) {
	siteID, err := c.resolveSiteID(ctx)
	if err != nil {
		return DNSPolicy{}, err
	}

	url := fmt.Sprintf("%s/sites/%s/dns/policies", c.BaseURL, siteID)
	body, err := json.Marshal(policy)
	if err != nil {
		return DNSPolicy{}, err
	}

	resp, err := c.doRequest(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return DNSPolicy{}, fmt.Errorf("failed to create policy: %w", err)
	}

	var created DNSPolicy
	if err := json.Unmarshal(resp, &created); err != nil {
		return DNSPolicy{}, err
	}
	return created, nil
}

func (c *Client) updateRecord(ctx context.Context, id string, policy DNSPolicy) (DNSPolicy, error) {
	siteID, err := c.resolveSiteID(ctx)
	if err != nil {
		return DNSPolicy{}, err
	}

	url := fmt.Sprintf("%s/sites/%s/dns/policies/%s", c.BaseURL, siteID, id)
	body, err := json.Marshal(policy)
	if err != nil {
		return DNSPolicy{}, err
	}

	resp, err := c.doRequest(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return DNSPolicy{}, fmt.Errorf("failed to update policy: %w", err)
	}

	var updated DNSPolicy
	if err := json.Unmarshal(resp, &updated); err != nil {
		return DNSPolicy{}, err
	}
	return updated, nil
}

func (c *Client) deleteRecord(ctx context.Context, id string) error {
	siteID, err := c.resolveSiteID(ctx)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/sites/%s/dns/policies/%s", c.BaseURL, siteID, id)
	_, err = c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete policy: %w", err)
	}
	return nil
}
