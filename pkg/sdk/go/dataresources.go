package nest

import (
	"context"
	"fmt"
)

// DataResourceSpec defines a new data resource to create.
type DataResourceSpec struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"` // postgres, mysql, kafka, s3, etc.
	Class  string            `json:"class"`
	Tenant string            `json:"tenant"`
	Labels map[string]string `json:"labels,omitempty"`
}

// DataResource is the resource as returned by the API.
type DataResource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Class    string `json:"class"`
	Tenant   string `json:"tenant"`
	Status   string `json:"status"`
	Endpoint string `json:"endpoint,omitempty"`
}

// DataResourceClient manages DataResource operations.
type DataResourceClient struct{ c *Client }

// List returns all data resources for a tenant.
func (d *DataResourceClient) List(ctx context.Context, tenant string) ([]DataResource, error) {
	resp, err := d.c.do(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%s/dataresources", tenant), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		DataResources []DataResource `json:"dataresources"`
	}
	return out.DataResources, decode(resp, &out)
}

// Get returns a single data resource by name.
func (d *DataResourceClient) Get(ctx context.Context, tenant, name string) (*DataResource, error) {
	resp, err := d.c.do(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%s/dataresources/%s", tenant, name), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		DataResource DataResource `json:"dataresource"`
	}
	return &out.DataResource, decode(resp, &out)
}

// Create creates a new data resource. Returns operation ID (async, 202).
func (d *DataResourceClient) Create(ctx context.Context, tenant string, spec DataResourceSpec) (string, error) {
	spec.Tenant = tenant
	resp, err := d.c.do(ctx, "POST", fmt.Sprintf("/api/v1/tenants/%s/dataresources", tenant), spec)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return resp.Header.Get("X-Operation-ID"), nil
}

// Delete deletes a data resource. Returns operation ID (async, 202).
func (d *DataResourceClient) Delete(ctx context.Context, tenant, name string) (string, error) {
	resp, err := d.c.do(ctx, "DELETE", fmt.Sprintf("/api/v1/tenants/%s/dataresources/%s", tenant, name), nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return resp.Header.Get("X-Operation-ID"), nil
}
