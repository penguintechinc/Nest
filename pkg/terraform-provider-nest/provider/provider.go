package provider

// NestProvider defines the Terraform provider schema.
// Full implementation requires github.com/hashicorp/terraform-plugin-framework.

// ProviderConfig holds connection settings from Terraform configuration.
type ProviderConfig struct {
	Endpoint string
	Token    string
	Tenant   string
}

// DataResourceState mirrors the nest_dataresource Terraform resource state.
type DataResourceState struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Class    string `json:"class"`
	Tenant   string `json:"tenant"`
	Status   string `json:"status"`
	Endpoint string `json:"endpoint"`
}

// Example Terraform HCL (documentation):
//
// provider "nest" {
//   endpoint = "https://nest.acme.com"
//   token    = var.nest_token
//   tenant   = "acme"
// }
//
// resource "nest_dataresource" "postgres" {
//   name   = "my-postgres"
//   type   = "postgres"
//   class  = "postgres-ha-3"
// }
//
// output "endpoint" {
//   value = nest_dataresource.postgres.endpoint
// }
