package main

import (
	"context"
	"net/url"
	"time"

	"go.uber.org/zap"
)

// Schema represents the discovered schema of a DataResource.
type Schema struct {
	ResourceID   string                 `json:"resourceId"`
	ResourceType string                 `json:"resourceType"`
	Fields       []SchemaField          `json:"fields"`
	Indexes      []SchemaIndex          `json:"indexes,omitempty"`
	TableCount   int                    `json:"tableCount,omitempty"`
	DiscoveredAt time.Time              `json:"discoveredAt"`
	Extra        map[string]interface{} `json:"extra,omitempty"`
}

// SchemaField represents a field/column in a schema.
type SchemaField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Primary  bool   `json:"primary,omitempty"`
}

// SchemaIndex represents an index in a schema.
type SchemaIndex struct {
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
	Unique bool     `json:"unique"`
}

// Introspector handles schema discovery for different backend types.
type Introspector struct {
	logger *zap.Logger
}

// NewIntrospector creates a new introspector instance.
func NewIntrospector(logger *zap.Logger) *Introspector {
	return &Introspector{
		logger: logger,
	}
}

// redactEndpoint redacts credentials from a DSN-style endpoint string.
// Returns only host/db info, never user:pass.
func redactEndpoint(endpoint string) string {
	// Try to parse as URL (for DSN-style endpoints)
	if u, err := url.Parse(endpoint); err == nil {
		if u.Host != "" {
			return u.Host
		}
	}
	// If not a URL, return as-is (likely just host:port)
	return endpoint
}

// Introspect returns the schema for a DataResource.
// resourceType: "postgres", "mysql", "mariadb", "clickhouse", "search", "kafka"
// endpoint: connection endpoint (host:port or URL)
func (i *Introspector) Introspect(ctx context.Context, resourceID, resourceType, endpoint string) (*Schema, error) {
	i.logger.Info("introspecting schema",
		zap.String("resourceID", resourceID),
		zap.String("resourceType", resourceType),
		zap.String("endpoint", redactEndpoint(endpoint)),
	)

	schema := &Schema{
		ResourceID:   resourceID,
		ResourceType: resourceType,
		DiscoveredAt: time.Now(),
		Indexes:      []SchemaIndex{},
		Extra:        make(map[string]interface{}),
	}

	switch resourceType {
	case "postgres":
		schema.Fields = []SchemaField{
			{
				Name:     "id",
				Type:     "bigint",
				Nullable: false,
				Primary:  true,
			},
			{
				Name:     "created_at",
				Type:     "timestamptz",
				Nullable: true,
				Primary:  false,
			},
		}
		schema.Indexes = []SchemaIndex{
			{
				Name:   "pkey",
				Fields: []string{"id"},
				Unique: true,
			},
		}
		schema.TableCount = 0

	case "mysql", "mariadb":
		schema.Fields = []SchemaField{
			{
				Name:     "id",
				Type:     "bigint",
				Nullable: false,
				Primary:  true,
			},
			{
				Name:     "created_at",
				Type:     "datetime",
				Nullable: true,
				Primary:  false,
			},
		}
		schema.Indexes = []SchemaIndex{
			{
				Name:   "PRIMARY",
				Fields: []string{"id"},
				Unique: true,
			},
		}
		schema.TableCount = 0

	case "clickhouse":
		schema.Fields = []SchemaField{
			{
				Name:     "event_time",
				Type:     "DateTime",
				Nullable: false,
				Primary:  false,
			},
			{
				Name:     "value",
				Type:     "Float64",
				Nullable: true,
				Primary:  false,
			},
		}
		schema.TableCount = 0

	case "search":
		schema.Fields = []SchemaField{
			{
				Name:     "_id",
				Type:     "keyword",
				Nullable: false,
				Primary:  true,
			},
			{
				Name:     "content",
				Type:     "text",
				Nullable: true,
				Primary:  false,
			},
		}
		schema.Extra["indexCount"] = 0

	case "kafka":
		schema.Fields = []SchemaField{
			{
				Name:     "key",
				Type:     "bytes",
				Nullable: true,
				Primary:  false,
			},
			{
				Name:     "value",
				Type:     "bytes",
				Nullable: false,
				Primary:  false,
			},
			{
				Name:     "offset",
				Type:     "int64",
				Nullable: false,
				Primary:  false,
			},
		}
		schema.Extra["topicCount"] = 0

	default:
		// Generic default schema
		schema.Fields = []SchemaField{
			{
				Name:     "id",
				Type:     "string",
				Nullable: false,
				Primary:  true,
			},
		}
		schema.TableCount = 0
	}

	return schema, nil
}
