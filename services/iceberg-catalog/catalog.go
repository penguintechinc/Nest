package main

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Namespace is a per-tenant catalog namespace.
type Namespace struct {
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
	CreatedAt  time.Time         `json:"created_at"`
}

// Table represents an Iceberg table registration.
type Table struct {
	Namespace        string                 `json:"namespace"`
	Name             string                 `json:"name"`
	Location         string                 `json:"location"`
	Schema           map[string]interface{} `json:"schema"`
	Properties       map[string]string      `json:"properties"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	MetadataLocation string                 `json:"metadata_location"`
}

// TableID uniquely identifies a table.
type TableID struct {
	Namespace string
	Name      string
}

// Catalog is the in-memory Iceberg REST catalog.
type Catalog struct {
	mu         sync.RWMutex
	namespaces map[string]*Namespace
	tables     map[TableID]*Table
	logger     *zap.Logger
}

// NewCatalog creates a new in-memory catalog.
func NewCatalog(logger *zap.Logger) *Catalog {
	return &Catalog{
		namespaces: make(map[string]*Namespace),
		tables:     make(map[TableID]*Table),
		logger:     logger,
	}
}

// CreateNamespace creates a new namespace. Returns error if already exists.
func (c *Catalog) CreateNamespace(name string, props map[string]string) (*Namespace, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.namespaces[name]; exists {
		return nil, fmt.Errorf("namespace already exists: %s", name)
	}

	if props == nil {
		props = make(map[string]string)
	}

	ns := &Namespace{
		Name:       name,
		Properties: props,
		CreatedAt:  time.Now().UTC(),
	}

	c.namespaces[name] = ns
	c.logger.Info("namespace created", zap.String("namespace", name))
	return ns, nil
}

// ListNamespaces lists all namespaces, optionally filtered by parent prefix.
func (c *Catalog) ListNamespaces(parent string) []*Namespace {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []*Namespace
	for _, ns := range c.namespaces {
		if parent == "" || ns.Name == parent {
			result = append(result, ns)
		}
	}
	return result
}

// GetNamespace gets a namespace by name.
func (c *Catalog) GetNamespace(name string) (*Namespace, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ns, exists := c.namespaces[name]
	if !exists {
		return nil, fmt.Errorf("namespace not found: %s", name)
	}
	return ns, nil
}

// DropNamespace drops a namespace (must be empty).
func (c *Catalog) DropNamespace(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.namespaces[name]; !exists {
		return fmt.Errorf("namespace not found: %s", name)
	}

	// Check if namespace is empty (no tables)
	for tableID := range c.tables {
		if tableID.Namespace == name {
			return fmt.Errorf("namespace not empty: %s", name)
		}
	}

	delete(c.namespaces, name)
	c.logger.Info("namespace dropped", zap.String("namespace", name))
	return nil
}

// CreateTable creates a new table in a namespace.
func (c *Catalog) CreateTable(ns, name, location string, schema map[string]interface{}, props map[string]string) (*Table, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Verify namespace exists
	if _, exists := c.namespaces[ns]; !exists {
		return nil, fmt.Errorf("namespace not found: %s", ns)
	}

	tableID := TableID{Namespace: ns, Name: name}
	if _, exists := c.tables[tableID]; exists {
		return nil, fmt.Errorf("table already exists: %s.%s", ns, name)
	}

	if props == nil {
		props = make(map[string]string)
	}

	if schema == nil {
		schema = make(map[string]interface{})
	}

	now := time.Now().UTC()
	table := &Table{
		Namespace:        ns,
		Name:             name,
		Location:         location,
		Schema:           schema,
		Properties:       props,
		CreatedAt:        now,
		UpdatedAt:        now,
		MetadataLocation: fmt.Sprintf("%s/.iceberg/metadata/v0.json", location),
	}

	c.tables[tableID] = table
	c.logger.Info("table created", zap.String("namespace", ns), zap.String("table", name))
	return table, nil
}

// ListTables lists all tables in a namespace.
func (c *Catalog) ListTables(ns string) ([]*Table, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Verify namespace exists
	if _, exists := c.namespaces[ns]; !exists {
		return nil, fmt.Errorf("namespace not found: %s", ns)
	}

	var result []*Table
	for tableID, table := range c.tables {
		if tableID.Namespace == ns {
			result = append(result, table)
		}
	}
	return result, nil
}

// GetTable gets a table by namespace + name.
func (c *Catalog) GetTable(ns, name string) (*Table, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tableID := TableID{Namespace: ns, Name: name}
	table, exists := c.tables[tableID]
	if !exists {
		return nil, fmt.Errorf("table not found: %s.%s", ns, name)
	}
	return table, nil
}

// DropTable drops a table.
func (c *Catalog) DropTable(ns, name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	tableID := TableID{Namespace: ns, Name: name}
	if _, exists := c.tables[tableID]; !exists {
		return fmt.Errorf("table not found: %s.%s", ns, name)
	}

	delete(c.tables, tableID)
	c.logger.Info("table dropped", zap.String("namespace", ns), zap.String("table", name))
	return nil
}

// UpdateTable updates table metadata (location, schema, properties).
func (c *Catalog) UpdateTable(ns, name string, updates map[string]interface{}) (*Table, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tableID := TableID{Namespace: ns, Name: name}
	table, exists := c.tables[tableID]
	if !exists {
		return nil, fmt.Errorf("table not found: %s.%s", ns, name)
	}

	if loc, ok := updates["location"].(string); ok {
		table.Location = loc
	}

	if sch, ok := updates["schema"].(map[string]interface{}); ok {
		table.Schema = sch
	}

	if props, ok := updates["properties"].(map[string]string); ok {
		for k, v := range props {
			table.Properties[k] = v
		}
	}

	table.UpdatedAt = time.Now().UTC()
	c.logger.Info("table updated", zap.String("namespace", ns), zap.String("table", name))
	return table, nil
}
