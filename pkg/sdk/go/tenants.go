package nest

import "context"

type Tenant struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type TenantClient struct{ c *Client }

func (t *TenantClient) List(ctx context.Context) ([]Tenant, error) {
	resp, err := t.c.do(ctx, "GET", "/api/v1/tenants", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Tenants []Tenant `json:"tenants"`
	}
	return out.Tenants, decode(resp, &out)
}
