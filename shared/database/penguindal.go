package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PenguinDAL provides a PyDAL-inspired API on top of GORM.
// This allows for a consistent data access pattern across Penguin Tech projects.
type PenguinDAL struct {
	db *gorm.DB
}

// NewPenguinDAL creates a new PenguinDAL instance.
func NewPenguinDAL(db *gorm.DB) *PenguinDAL {
	return &PenguinDAL{db: db}
}

// DB returns the underlying GORM database instance.
func (d *PenguinDAL) DB() *gorm.DB {
	return d.db
}

// DefineTable is a placeholder for PyDAL-style table definition.
// In Go, we use structs, so this mostly ensures migrations are run.
func (d *PenguinDAL) DefineTable(model interface{}) error {
	return d.db.AutoMigrate(model)
}

// Insert adds a new record to the database.
func (d *PenguinDAL) Insert(model interface{}) error {
	return d.db.Create(model).Error
}

// Get retrieves a single record by its primary key.
func (d *PenguinDAL) Get(model interface{}, id interface{}) error {
	return d.db.First(model, id).Error
}

// Update updates an existing record.
func (d *PenguinDAL) Update(model interface{}) error {
	return d.db.Save(model).Error
}

// Delete removes a record from the database (soft delete if DeletedAt is present).
func (d *PenguinDAL) Delete(model interface{}, id interface{}) error {
	return d.db.Delete(model, id).Error
}

// Query provides a PyDAL-like query interface.
func (d *PenguinDAL) Query() *gorm.DB {
	return d.db
}

// Table returns a GORM session for a specific table.
func (d *PenguinDAL) Table(name string) *gorm.DB {
	return d.db.Table(name)
}

// Transaction executes a function within a database transaction.
func (d *PenguinDAL) Transaction(fn func(tx *gorm.DB) error) error {
	return d.db.Transaction(fn)
}

// Now returns the current time in UTC.
func (d *PenguinDAL) Now() time.Time {
	return time.Now().UTC()
}

// UUID generates a unique string identifier.
func (d *PenguinDAL) UUID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
