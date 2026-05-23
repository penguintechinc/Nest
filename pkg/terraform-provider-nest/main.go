// Package main is the Terraform provider binary entry point.
// This is a stub; full implementation uses HashiCorp Plugin Framework.
package main

import (
	"fmt"
	"os"
)

// Version is the provider version.
const Version = "1.0.0"

func main() {
	// Terraform providers are started as gRPC plugin servers.
	// This stub prints usage and exits; real implementation uses:
	//   plugin.Serve(&plugin.ServeOpts{ProviderFunc: provider.New})
	fmt.Fprintln(os.Stderr, "nest terraform provider v"+Version)
	fmt.Fprintln(os.Stderr, "This binary is invoked by Terraform; do not run directly.")
	fmt.Fprintln(os.Stderr, "Resources: nest_dataresource, nest_credential, nest_hardware_pool")
	os.Exit(1)
}
