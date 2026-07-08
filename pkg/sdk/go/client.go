// Package nest provides the Go client SDK for the Nest storage platform.
package nest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is the Nest API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	// Resource sub-clients
	DataResources *DataResourceClient
	Databases     *DatabaseClient
	Tenants       *TenantClient
	Credentials   *CredentialClient
}

// NewClient creates a new Nest client.
// baseURL example: "https://nest.acme.com"
// token: JWT or API key in Authorization: Bearer header
func NewClient(baseURL, token string) *Client {
	c := &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	c.DataResources = &DataResourceClient{c}
	c.Databases = &DatabaseClient{c}
	c.Tenants = &TenantClient{c}
	c.Credentials = &CredentialClient{c}
	return c
}

// do executes an authenticated HTTP request.
func (c *Client) do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

// decode reads a JSON response into v.
func decode(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return fmt.Errorf("api error %d: %s", resp.StatusCode, apiErr.Error)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
