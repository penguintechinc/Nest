package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/penguintechinc/nest/tools/nestctl/migrate"
	"github.com/penguintechinc/nest/tools/nestctl/resource"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "migrate":
		if len(os.Args) < 3 {
			printUsage()
			os.Exit(1)
		}
		handleMigrate(os.Args[2:])
	case "resource":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: nestctl resource <list|get|create|delete> [flags]")
			os.Exit(1)
		}
		handleResource(os.Args[2:])
	case "config":
		fmt.Println("Configuration: use NEST_ENDPOINT, NEST_TOKEN, NEST_TENANT env vars")
		os.Exit(0)
	case "help", "-h", "--help":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handleMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)

	subcommand := ""
	if len(args) > 0 {
		subcommand = args[0]
		args = args[1:]
	}

	switch subcommand {
	case "longhorn":
		handleMigrateLonghorn(args)
	case "":
		fmt.Fprintf(os.Stderr, "Usage: nestctl migrate <subcommand> [flags]\n")
		fmt.Fprintf(os.Stderr, "Subcommands:\n")
		fmt.Fprintf(os.Stderr, "  longhorn    Migrate Longhorn volumes to Nest DataResources\n")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "Unknown migrate subcommand: %s\n", subcommand)
		os.Exit(1)
	}

	_ = fs // Silence unused variable warning
}

func handleMigrateLonghorn(args []string) {
	fs := flag.NewFlagSet("migrate longhorn", flag.ExitOnError)

	namespace := fs.String("namespace", "", "Kubernetes namespace to scan (default: all namespaces)")
	tenant := fs.String("tenant", "", "Target Nest tenant (required)")
	output := fs.String("output", "./nest-migration", "Output directory for DataResource YAMLs")
	dryRun := fs.Bool("dry-run", false, "Print plan without writing files")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	// Validate required flags
	if *tenant == "" {
		fmt.Fprintf(os.Stderr, "Error: --tenant flag is required\n")
		os.Exit(1)
	}

	// Create context
	ctx := context.Background()

	// Discover Longhorn volumes
	fmt.Printf("Discovering Longhorn volumes in namespace '%s'...\n", *namespace)
	if *namespace == "" {
		fmt.Println("  (scanning all namespaces)")
	}

	volumes, err := migrate.DiscoverLonghornVolumes(ctx, *namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering Longhorn volumes: %v\n", err)
		os.Exit(1)
	}

	if len(volumes) == 0 {
		fmt.Println("No Longhorn volumes found.")
		os.Exit(0)
	}

	fmt.Printf("Found %d Longhorn volume(s):\n", len(volumes))
	for _, vol := range volumes {
		sizeGi := vol.SizeBytes / (1024 * 1024 * 1024)
		fmt.Printf("  - %s/%s (%dGi, StorageClass: %s)\n",
			vol.Namespace, vol.Name, sizeGi, vol.StorageClass)
	}

	// Plan migration
	fmt.Println()
	fmt.Printf("Planning migration to tenant '%s'...\n", *tenant)
	plan := migrate.PlanMigration(volumes, *tenant, *dryRun)

	// Execute migration
	result, err := migrate.ExecuteMigration(ctx, plan, *output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing migration: %v\n", err)
		os.Exit(1)
	}

	// Print results
	fmt.Println()
	fmt.Println("Migration Results:")
	fmt.Printf("  Created:  %d file(s)\n", len(result.Created))
	for _, filename := range result.Created {
		fmt.Printf("    - %s\n", filename)
	}

	if len(result.Failed) > 0 {
		fmt.Printf("  Failed:   %d\n", len(result.Failed))
		for vol, err := range result.Errors {
			fmt.Printf("    - %s: %v\n", vol, err)
		}
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Migration complete!")
	if *dryRun {
		fmt.Println("This was a dry-run. To execute the migration, run without --dry-run flag.")
	} else {
		fmt.Printf("DataResource YAMLs written to: %s\n", *output)
		fmt.Println("Review the generated files and apply them manually using:")
		fmt.Printf("  kubectl apply -f %s/\n", *output)
	}
}

func handleResource(args []string) {
	fs := flag.NewFlagSet("resource", flag.ExitOnError)
	endpoint := fs.String("endpoint", "", "Nest API endpoint")
	token := fs.String("token", "", "API token")
	tenant := fs.String("tenant", "", "Tenant name")
	output := fs.String("output", "table", "Output format: table, json")
	rtype := fs.String("type", "", "Resource type filter")
	class := fs.String("class", "", "Resource class")

	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: nestctl resource <list|get|create|delete> [flags]")
		os.Exit(1)
	}

	subcommand := args[0]
	_ = fs.Parse(args[1:])

	cfg := resource.LoadConfig(*endpoint, *token, *tenant, *output)

	var err error
	switch subcommand {
	case "list":
		err = resource.ListResources(cfg, *rtype)
	case "get":
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Usage: nestctl resource get <name>")
			os.Exit(1)
		}
		err = resource.GetResource(cfg, fs.Arg(0))
	case "create":
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Usage: nestctl resource create <name> --type=X --class=Y")
			os.Exit(1)
		}
		err = resource.CreateResource(cfg, fs.Arg(0), *rtype, *class)
	case "delete":
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Usage: nestctl resource delete <name>")
			os.Exit(1)
		}
		err = resource.DeleteResource(cfg, fs.Arg(0))
	default:
		fmt.Fprintf(os.Stderr, "Unknown resource subcommand: %s\n", subcommand)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `nestctl - Nest cluster management CLI

Usage:
  nestctl <command> [flags]

Commands:
  migrate       Migration utilities
    longhorn    Migrate Longhorn volumes to Nest DataResources
  resource      Manage DataResources
    list        List resources for a tenant
    get         Get a single resource
    create      Create a new resource
    delete      Delete a resource
  config        Show configuration
  help          Print this help message

Examples:
  nestctl migrate longhorn --tenant mycompany
  nestctl resource list --endpoint https://nest.acme.com --token sk-... --tenant acme
  nestctl resource create my-db --type postgres --class standard --endpoint https://nest.acme.com --token sk-... --tenant acme

Environment Variables:
  NEST_ENDPOINT    API endpoint (required for resource commands)
  NEST_TOKEN       API token (required for resource commands)
  NEST_TENANT      Default tenant name

Flags for 'migrate longhorn':
  --namespace string    Kubernetes namespace to scan (default: all namespaces)
  --tenant string       Target Nest tenant (required)
  --output string       Output directory for DataResource YAMLs (default: ./nest-migration)
  --dry-run             Print plan without writing files

Flags for 'resource':
  --endpoint string     API endpoint
  --token string        API token
  --tenant string       Tenant name
  --output string       Output format: table, json (default: table)
  --type string         Resource type filter (for list)
  --class string        Resource class (for create)

`)
}
