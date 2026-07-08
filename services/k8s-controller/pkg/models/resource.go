package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// JSONMap represents a JSON object stored in database
type JSONMap map[string]interface{}

// Scan implements sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// Value implements driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Resource represents a managed resource in the NEST database
type Resource struct {
	ID                 uint    `gorm:"primaryKey"`
	Name               string  `gorm:"size:255;not null"`
	ResourceTypeID     uint    `gorm:"not null"`
	TeamID             uint    `gorm:"not null;index"`
	Status             string  `gorm:"size:50;default:pending"`
	LifecycleMode      string  `gorm:"size:50;not null"`
	ProvisioningMethod *string `gorm:"size:50"`
	ConnectionInfo     JSONMap `gorm:"type:jsonb"`
	Credentials        JSONMap `gorm:"type:jsonb"`
	TLSEnabled         bool    `gorm:"default:false"`
	TLSCaID            *uint
	TLSCertID          *uint
	K8sNamespace       *string `gorm:"size:255"`
	K8sResourceName    *string `gorm:"size:255"`
	K8sResourceType    *string `gorm:"size:50"`
	Config             JSONMap `gorm:"type:jsonb"`
	CanModifyUsers     bool    `gorm:"default:false"`
	CanModifyConfig    bool    `gorm:"default:false"`
	CanBackup          bool    `gorm:"default:false"`
	CanScale           bool    `gorm:"default:false"`
	CreatedBy          *uint
	CreatedAt          time.Time  `gorm:"autoCreateTime"`
	UpdatedAt          time.Time  `gorm:"autoUpdateTime"`
	DeletedAt          *time.Time `gorm:"index"`
}

// TableName specifies the table name for Resource
func (Resource) TableName() string {
	return "resources"
}

// ResourceType represents different types of resources that can be managed
type ResourceType struct {
	ID                       uint   `gorm:"primaryKey"`
	Name                     string `gorm:"size:100;uniqueIndex;not null"`
	Category                 string `gorm:"size:50;not null"`
	DisplayName              string `gorm:"size:255;not null"`
	Icon                     string `gorm:"size:100"`
	SupportsFullLifecycle    bool   `gorm:"default:true"`
	SupportsPartialLifecycle bool   `gorm:"default:true"`
	SupportsUserManagement   bool   `gorm:"default:false"`
	SupportsBackup           bool   `gorm:"default:false"`
	CreatedAt                time.Time
}

// TableName specifies the table name for ResourceType
func (ResourceType) TableName() string {
	return "resource_types"
}

// ProvisioningJob represents a provisioning operation
type ProvisioningJob struct {
	ID           uint   `gorm:"primaryKey"`
	ResourceID   uint   `gorm:"not null;index"`
	JobType      string `gorm:"size:50;not null"`
	Status       string `gorm:"size:50;default:pending"`
	StartedAt    *time.Time
	CompletedAt  *time.Time
	Logs         *string `gorm:"type:text"`
	ErrorMessage *string `gorm:"type:text"`
	CreatedBy    *uint
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

// TableName specifies the table name for ProvisioningJob
func (ProvisioningJob) TableName() string {
	return "provisioning_jobs"
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID           uint      `gorm:"primaryKey"`
	UserID       *uint     `gorm:"index"`
	Action       string    `gorm:"size:100;not null"`
	ResourceType *string   `gorm:"size:100"`
	ResourceID   *uint     `gorm:"index"`
	TeamID       *uint     `gorm:"index"`
	Details      JSONMap   `gorm:"type:jsonb"`
	IPAddress    *string   `gorm:"size:45"`
	UserAgent    *string   `gorm:"type:text"`
	Timestamp    time.Time `gorm:"autoCreateTime;index"`
}

// TableName specifies the table name for AuditLog
func (AuditLog) TableName() string {
	return "audit_logs"
}
