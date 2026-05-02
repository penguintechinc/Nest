package nest

import (
	"context"
	"fmt"
)

type Credential struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ResourceID string `json:"resourceId"`
	Tenant     string `json:"tenant"`
	Username   string `json:"username"`
	Endpoint   string `json:"endpoint,omitempty"`
}

type CredentialClient struct{ c *Client }

func (cr *CredentialClient) List(ctx context.Context, tenant string) ([]Credential, error) {
	resp, err := cr.c.do(ctx, "GET", fmt.Sprintf("/api/v1/tenants/%s/credentials", tenant), nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Credentials []Credential `json:"credentials"`
	}
	return out.Credentials, decode(resp, &out)
}
