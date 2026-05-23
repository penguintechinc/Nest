package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"
)

// Config holds CLI settings loaded from env or flags
type Config struct {
	Endpoint string // NEST_ENDPOINT or --endpoint flag
	Token    string // NEST_TOKEN or --token flag
	Tenant   string // NEST_TENANT or --tenant flag
	Output   string // --output flag: table, json, yaml
}

// LoadConfig reads from env vars with overrides
func LoadConfig(endpoint, token, tenant, output string) Config {
	cfg := Config{
		Endpoint: endpoint,
		Token:    token,
		Tenant:   tenant,
		Output:   output,
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = os.Getenv("NEST_ENDPOINT")
	}
	if cfg.Token == "" {
		cfg.Token = os.Getenv("NEST_TOKEN")
	}
	if cfg.Tenant == "" {
		cfg.Tenant = os.Getenv("NEST_TENANT")
	}
	if cfg.Output == "" {
		cfg.Output = "table"
	}
	return cfg
}

// doRequest sends an authenticated HTTP request
func doRequest(cfg Config, method, path string, body interface{}) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, cfg.Endpoint+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

// PrintTable prints a list of resources as a table
func PrintTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, joinTab(headers))
	for _, row := range rows {
		fmt.Fprintln(w, joinTab(row))
	}
	w.Flush()
}

func joinTab(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\t"
		}
		result += p
	}
	return result
}

// ListResources lists dataresources for a tenant
func ListResources(cfg Config, resourceType string) error {
	path := fmt.Sprintf("/api/v1/tenants/%s/dataresources", cfg.Tenant)
	if resourceType != "" {
		path += "?type=" + resourceType
	}
	resp, err := doRequest(cfg, "GET", path, nil)
	if err != nil {
		return fmt.Errorf("list resources: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		DataResources []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Class    string `json:"class"`
			Status   string `json:"status"`
			Endpoint string `json:"endpoint"`
		} `json:"dataresources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	if cfg.Output == "json" {
		return json.NewEncoder(os.Stdout).Encode(data)
	}

	rows := make([][]string, len(data.DataResources))
	for i, r := range data.DataResources {
		rows[i] = []string{r.Name, r.Type, r.Class, r.Status, r.Endpoint}
	}
	PrintTable([]string{"NAME", "TYPE", "CLASS", "STATUS", "ENDPOINT"}, rows)
	return nil
}

// GetResource gets a single dataresource
func GetResource(cfg Config, name string) error {
	resp, err := doRequest(cfg, "GET", fmt.Sprintf("/api/v1/tenants/%s/dataresources/%s", cfg.Tenant, name), nil)
	if err != nil {
		return fmt.Errorf("get resource: %w", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(data)
}

// CreateResource creates a new dataresource
func CreateResource(cfg Config, name, resourceType, class string) error {
	body := map[string]string{
		"name":   name,
		"type":   resourceType,
		"class":  class,
		"tenant": cfg.Tenant,
	}
	resp, err := doRequest(cfg, "POST", fmt.Sprintf("/api/v1/tenants/%s/dataresources", cfg.Tenant), body)
	if err != nil {
		return fmt.Errorf("create resource: %w", err)
	}
	defer resp.Body.Close()

	opID := resp.Header.Get("X-Operation-ID")
	fmt.Printf("Creating resource %q (type: %s, class: %s)\n", name, resourceType, class)
	if opID != "" {
		fmt.Printf("Operation: %s\n", opID)
	}
	return nil
}

// DeleteResource deletes a dataresource
func DeleteResource(cfg Config, name string) error {
	resp, err := doRequest(cfg, "DELETE", fmt.Sprintf("/api/v1/tenants/%s/dataresources/%s", cfg.Tenant, name), nil)
	if err != nil {
		return fmt.Errorf("delete resource: %w", err)
	}
	defer resp.Body.Close()

	opID := resp.Header.Get("X-Operation-ID")
	fmt.Printf("Deleting resource %q\n", name)
	if opID != "" {
		fmt.Printf("Operation: %s\n", opID)
	}
	return nil
}
