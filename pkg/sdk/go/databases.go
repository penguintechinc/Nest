package nest

import (
	"context"
	"fmt"
)

type DatabaseSpec struct {
	Name   string `json:"name"`
	Type   string `json:"type"`   // postgres, mysql, mariadb
	Class  string `json:"class"`
	Tenant string `json:"tenant"`
}

type Database struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Class    string `json:"class"`
	Tenant   string `json:"tenant"`
	Endpoint string `json:"endpoint,omitempty"`
	Status   string `json:"status"`
}

type DatabaseClient struct{ c *Client }

func (d *DatabaseClient) List(ctx context.Context, tenant string) ([]Database, error) {
	resp, err := d.c.do(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%s/databases", tenant), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Databases []Database `json:"databases"`
	}
	return out.Databases, decode(resp, &out)
}

func (d *DatabaseClient) Create(ctx context.Context, tenant string, spec DatabaseSpec) (string, error) {
	spec.Tenant = tenant
	resp, err := d.c.do(ctx, "POST", fmt.Sprintf("/api/v1/tenants/%s/databases", tenant), spec)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return resp.Header.Get("X-Operation-ID"), nil
}
