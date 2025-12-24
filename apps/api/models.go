package main

import (
	"database/sql"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// BaseModel provides common fields for all models
type BaseModel struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// Team represents a team in the system
type Team struct {
	BaseModel
	Name        string       `gorm:"uniqueIndex;not null" json:"name"`
	Description string       `json:"description"`
	IsGlobal    bool         `gorm:"default:false" json:"is_global"`
	Members     []TeamMember `gorm:"foreignKey:TeamID" json:"members,omitempty"`
}

// ResourceType represents a type of resource
type ResourceType struct {
	BaseModel
	Name                     string `gorm:"uniqueIndex;not null" json:"name"`
	Category                 string `json:"category"`
	DisplayName              string `json:"display_name"`
	Icon                     string `json:"icon"`
	SupportsFullLifecycle    bool   `json:"supports_full_lifecycle"`
	SupportsPartialLifecycle bool   `json:"supports_partial_lifecycle"`
	SupportsUserManagement   bool   `json:"supports_user_management"`
	SupportsBackup           bool   `json:"supports_backup"`
}

// Resource represents a managed resource
type Resource struct {
	BaseModel
	Name               string         `gorm:"not null" json:"name"`
	ResourceTypeID     uint           `gorm:"not null" json:"resource_type_id"`
	ResourceType       *ResourceType  `gorm:"foreignKey:ResourceTypeID" json:"resource_type,omitempty"`
	TeamID             uint           `gorm:"not null;index" json:"team_id"`
	Team               *Team          `gorm:"foreignKey:TeamID" json:"team,omitempty"`
	Status             string         `gorm:"default:'pending'" json:"status"`
	LifecycleMode      string         `gorm:"not null" json:"lifecycle_mode"`
	ProvisioningMethod string         `json:"provisioning_method"`
	ConnectionInfo     datatypes.JSON `gorm:"type:jsonb" json:"connection_info"`
	Credentials        datatypes.JSON `gorm:"type:jsonb" json:"-"`
	TLSEnabled         bool           `gorm:"default:false" json:"tls_enabled"`
	TLSCertID          *uint          `json:"tls_cert_id"`
	K8sNamespace       string         `json:"k8s_namespace"`
	K8sResourceName    string         `json:"k8s_resource_name"`
	K8sResourceType    string         `json:"k8s_resource_type"`
	Config             datatypes.JSON `gorm:"type:jsonb" json:"config"`
	CanModifyUsers     bool           `gorm:"default:false" json:"can_modify_users"`
	CanModifyConfig    bool           `gorm:"default:false" json:"can_modify_config"`
	CanBackup          bool           `gorm:"default:false" json:"can_backup"`
	CanScale           bool           `gorm:"default:false" json:"can_scale"`
	CreatedBy          uint           `json:"created_by"`
}

// ResourceStats represents statistics for a resource
type ResourceStats struct {
	BaseModel
	ResourceID  uint           `gorm:"not null;index" json:"resource_id"`
	Resource    *Resource      `gorm:"foreignKey:ResourceID" json:"resource,omitempty"`
	Timestamp   time.Time      `gorm:"not null;index" json:"timestamp"`
	Metrics     datatypes.JSON `gorm:"type:jsonb" json:"metrics"`
	RiskLevel   string         `json:"risk_level"`
	RiskFactors datatypes.JSON `gorm:"type:jsonb" json:"risk_factors"`
}

// TableName specifies the table name for Resource
func (Resource) TableName() string {
	return "resources"
}

// TableName specifies the table name for ResourceStats
func (ResourceStats) TableName() string {
	return "resource_stats"
}

// User represents a system user
type User struct {
	BaseModel
	Username     string     `gorm:"uniqueIndex;not null" json:"username"`
	Email        string     `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash string     `gorm:"not null" json:"-"`
	FirstName    string     `json:"first_name"`
	LastName     string     `json:"last_name"`
	Role         string     `gorm:"default:'user'" json:"role"`
	IsActive     bool       `gorm:"default:true" json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// TeamMember represents membership in a team
type TeamMember struct {
	BaseModel
	TeamID uint   `gorm:"not null;uniqueIndex:idx_team_user" json:"team_id"`
	Team   *Team  `gorm:"foreignKey:TeamID" json:"team,omitempty"`
	UserID uint   `gorm:"not null;uniqueIndex:idx_team_user" json:"user_id"`
	User   *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Role   string `gorm:"not null" json:"role"`
}

// TableName specifies the table name for TeamMember
func (TeamMember) TableName() string {
	return "team_members"
}

// RBACContext holds RBAC information for the current user
type RBACContext struct {
	UserID     uint
	User       *User
	Teams      []*Team
	TeamRoles  map[uint]string
	GlobalRole string
}

// Request/Response DTOs

// CreateResourceRequest is the request body for creating a resource
type CreateResourceRequest struct {
	Name               string                 `json:"name" binding:"required"`
	ResourceTypeID     uint                   `json:"resource_type_id" binding:"required"`
	TeamID             uint                   `json:"team_id" binding:"required"`
	LifecycleMode      string                 `json:"lifecycle_mode" binding:"required,oneof=full partial monitor_only"`
	ProvisioningMethod string                 `json:"provisioning_method"`
	ConnectionInfo     map[string]interface{} `json:"connection_info"`
	Credentials        map[string]interface{} `json:"credentials"`
	Config             map[string]interface{} `json:"config"`
	TLSEnabled         bool                   `json:"tls_enabled"`
	Capabilities       map[string]bool        `json:"capabilities"`
}

// UpdateResourceRequest is the request body for updating a resource
type UpdateResourceRequest struct {
	Name   *string                `json:"name"`
	Status *string                `json:"status"`
	Config map[string]interface{} `json:"config"`
}

// ResourceResponse is the response body for a resource
type ResourceResponse struct {
	ID                 uint                   `json:"id"`
	Name               string                 `json:"name"`
	ResourceTypeID     uint                   `json:"resource_type_id"`
	ResourceType       *ResourceType          `json:"resource_type,omitempty"`
	TeamID             uint                   `json:"team_id"`
	Team               *Team                  `json:"team,omitempty"`
	Status             string                 `json:"status"`
	LifecycleMode      string                 `json:"lifecycle_mode"`
	ProvisioningMethod string                 `json:"provisioning_method"`
	ConnectionInfo     map[string]interface{} `json:"connection_info"`
	TLSEnabled         bool                   `json:"tls_enabled"`
	Config             map[string]interface{} `json:"config"`
	CanModifyUsers     bool                   `json:"can_modify_users"`
	CanModifyConfig    bool                   `json:"can_modify_config"`
	CanBackup          bool                   `json:"can_backup"`
	CanScale           bool                   `json:"can_scale"`
	CreatedBy          uint                   `json:"created_by"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	DeletedAt          sql.NullTime           `json:"deleted_at,omitempty"`
}

// ConnectionInfoResponse is the response for connection details
type ConnectionInfoResponse struct {
	ConnectionInfo map[string]interface{} `json:"connection_info"`
	Credentials    map[string]interface{} `json:"credentials,omitempty"`
	TLSEnabled     bool                   `json:"tls_enabled"`
	TLSCertID      *uint                  `json:"tls_cert_id,omitempty"`
	AccessLevel    string                 `json:"access_level"`
}

// ResourceStatsResponse is the response for resource statistics
type ResourceStatsResponse struct {
	ResourceID  uint                   `json:"resource_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Metrics     map[string]interface{} `json:"metrics"`
	RiskLevel   string                 `json:"risk_level"`
	RiskFactors map[string]interface{} `json:"risk_factors"`
}

// ResourceListResponse is the response for a list of resources
type ResourceListResponse struct {
	Resources []*ResourceResponse `json:"resources"`
	Total     int64               `json:"total"`
	Page      int                 `json:"page"`
	PageSize  int                 `json:"page_size"`
}

// ErrorResponse is a standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}
