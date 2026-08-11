package database

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ========== Core Models ==========
// These models extend/enhance the basic models defined in postgres.go
// Note: Team, User, TeamMember are defined in postgres.go with BaseModel
// This file adds extended models and relationships not in the base definitions

// TeamMembership represents membership of a user in a team (extended TeamMember model)
// Inherits from TeamMember but adds additional functionality
type TeamMembership struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	UserID    uint           `gorm:"not null;index:idx_team_user,unique" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
	TeamID    uint           `gorm:"not null;index:idx_team_user,unique" json:"team_id"`
	Team      Team           `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE" json:"team,omitempty"`
	Role      string         `gorm:"not null;default:team_viewer;size:50" json:"role"` // team_admin, team_maintainer, team_viewer
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName specifies the table name for TeamMembership
func (TeamMembership) TableName() string {
	return "team_memberships"
}

// ResourceType represents a type of resource (VM, Container, Database, etc.)
type ResourceType struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"uniqueIndex;not null;size:255" json:"name"`
	Category       string     `gorm:"not null;size:100" json:"category"` // compute, storage, networking, database
	DisplayName    string     `gorm:"size:255" json:"display_name"`
	Icon           string     `gorm:"size:500" json:"icon"`
	SupportsBackup bool       `gorm:"default:false" json:"supports_backup"`
	SupportsHA     bool       `gorm:"default:false" json:"supports_ha"`
	SupportsDR     bool       `gorm:"default:false" json:"supports_dr"`
	SupportsSSL    bool       `gorm:"default:false" json:"supports_ssl"`
	CreatedAt      time.Time  `json:"created_at"`
	Resources      []Resource `gorm:"foreignKey:ResourceTypeID;constraint:OnDelete:RESTRICT" json:"resources,omitempty"`
}

// TableName specifies the table name for ResourceType
func (ResourceType) TableName() string {
	return "resource_types"
}

// StringMap is a custom type for storing JSON data
type StringMap map[string]interface{}

// Value implements the driver.Valuer interface
func (m StringMap) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface
func (m *StringMap) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return gorm.ErrInvalidData
	}
	return json.Unmarshal(bytes, &m)
}

// Resource represents a managed resource
type Resource struct {
	ID                 uint              `gorm:"primaryKey" json:"id"`
	Name               string            `gorm:"not null;size:255" json:"name"`
	ResourceTypeID     uint              `gorm:"not null;index" json:"resource_type_id"`
	ResourceType       ResourceType      `gorm:"foreignKey:ResourceTypeID;constraint:OnDelete:RESTRICT" json:"resource_type,omitempty"`
	TeamID             uint              `gorm:"not null;index" json:"team_id"`
	Team               Team              `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE" json:"team,omitempty"`
	Status             string            `gorm:"not null;default:active;size:50" json:"status"` // active, inactive, provisioning, deprovisioning, error
	LifecycleMode      string            `gorm:"size:50" json:"lifecycle_mode"`                 // managed, unmanaged
	ProvisioningMethod string            `gorm:"size:100" json:"provisioning_method"`           // terraform, ansible, manual, api
	ConnectionInfo     datatypes.JSON    `gorm:"type:jsonb" json:"connection_info,omitempty"`
	Credentials        datatypes.JSON    `gorm:"type:jsonb" json:"credentials,omitempty"`
	TLSEnabled         bool              `gorm:"default:false" json:"tls_enabled"`
	TLSVerify          bool              `gorm:"default:true" json:"tls_verify"`
	TLSCertID          *uint             `json:"tls_cert_id,omitempty"`
	K8sClusterName     string            `gorm:"size:255" json:"k8s_cluster_name,omitempty"`
	K8sNamespace       string            `gorm:"size:255;default:default" json:"k8s_namespace,omitempty"`
	K8sIngressHost     string            `gorm:"size:255" json:"k8s_ingress_host,omitempty"`
	CanBackup          bool              `gorm:"default:false" json:"can_backup"`
	CanMonitor         bool              `gorm:"default:false" json:"can_monitor"`
	CanScale           bool              `gorm:"default:false" json:"can_scale"`
	CanMigrate         bool              `gorm:"default:false" json:"can_migrate"`
	Config             datatypes.JSON    `gorm:"type:jsonb" json:"config,omitempty"`
	CreatedBy          uint              `json:"created_by"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	DeletedAt          gorm.DeletedAt    `gorm:"index" json:"deleted_at,omitempty"`
	Users              []ResourceUser    `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"users,omitempty"`
	Certificates       []Certificate     `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"certificates,omitempty"`
	Stats              []ResourceStats   `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"stats,omitempty"`
	BackupJobs         []BackupJob       `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"backup_jobs,omitempty"`
	ProvisioningJobs   []ProvisioningJob `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"provisioning_jobs,omitempty"`
}

// TableName specifies the table name for Resource
func (Resource) TableName() string {
	return "resources"
}

// BeforeCreate hook for Resource
func (r *Resource) BeforeCreate(tx *gorm.DB) error {
	if r.Status == "" {
		r.Status = "active"
	}
	if r.K8sNamespace == "" {
		r.K8sNamespace = "default"
	}
	return nil
}

// ResourceUser represents a user account on a resource
type ResourceUser struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	ResourceID   uint           `gorm:"not null;index" json:"resource_id"`
	Resource     Resource       `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"resource,omitempty"`
	Username     string         `gorm:"not null;size:255" json:"username"`
	PasswordHash string         `gorm:"not null;size:255" json:"-"`
	Roles        datatypes.JSON `gorm:"type:jsonb" json:"roles,omitempty"`
	SyncStatus   string         `gorm:"size:50" json:"sync_status"` // synced, pending, failed
	LastSyncedAt *time.Time     `json:"last_synced_at,omitempty"`
	SyncError    string         `gorm:"type:text" json:"sync_error,omitempty"`
	CreatedBy    uint           `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName specifies the table name for ResourceUser
func (ResourceUser) TableName() string {
	return "resource_users"
}

// CertificateAuthority represents a Certificate Authority
type CertificateAuthority struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	Name          string         `gorm:"uniqueIndex;not null;size:255" json:"name"`
	Type          string         `gorm:"not null;size:50" json:"type"` // root, intermediate, self-signed
	Certificate   string         `gorm:"type:text;not null" json:"certificate"`
	PrivateKey    string         `gorm:"type:text;not null" json:"-"`
	Subject       string         `gorm:"size:255" json:"subject"`
	Issuer        string         `gorm:"size:255" json:"issuer"`
	ValidFrom     time.Time      `json:"valid_from"`
	ValidUntil    time.Time      `json:"valid_until"`
	SerialNumber  string         `gorm:"size:255" json:"serial_number"`
	IsNestManaged bool           `gorm:"default:true" json:"is_nest_managed"`
	CreatedBy     uint           `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Certificates  []Certificate  `gorm:"foreignKey:CAID;constraint:OnDelete:RESTRICT" json:"certificates,omitempty"`
}

// TableName specifies the table name for CertificateAuthority
func (CertificateAuthority) TableName() string {
	return "certificate_authorities"
}

// Certificate represents a certificate for TLS/SSL
type Certificate struct {
	ID                   uint                 `gorm:"primaryKey" json:"id"`
	ResourceID           *uint                `gorm:"index" json:"resource_id,omitempty"`
	Resource             *Resource            `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"resource,omitempty"`
	CAID                 uint                 `gorm:"not null;index" json:"ca_id"`
	CA                   CertificateAuthority `gorm:"foreignKey:CAID;constraint:OnDelete:RESTRICT" json:"ca,omitempty"`
	Certificate          string               `gorm:"type:text;not null" json:"certificate"`
	PrivateKey           string               `gorm:"type:text;not null" json:"-"`
	CommonName           string               `gorm:"not null;size:255" json:"common_name"`
	SANDns               datatypes.JSON       `gorm:"type:jsonb" json:"san_dns,omitempty"`
	SANIPs               datatypes.JSON       `gorm:"type:jsonb" json:"san_ips,omitempty"`
	ValidFrom            time.Time            `json:"valid_from"`
	ValidUntil           time.Time            `json:"valid_until"`
	SerialNumber         string               `gorm:"size:255" json:"serial_number"`
	AutoRenew            bool                 `gorm:"default:true" json:"auto_renew"`
	RenewalThresholdDays int                  `gorm:"default:30" json:"renewal_threshold_days"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
	DeletedAt            gorm.DeletedAt       `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName specifies the table name for Certificate
func (Certificate) TableName() string {
	return "certificates"
}

// ResourceStats represents statistics for a resource
type ResourceStats struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	ResourceID  uint           `gorm:"not null;index" json:"resource_id"`
	Resource    Resource       `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"resource,omitempty"`
	Timestamp   time.Time      `gorm:"index" json:"timestamp"`
	Metrics     datatypes.JSON `gorm:"type:jsonb" json:"metrics,omitempty"`
	RiskLevel   string         `gorm:"size:50" json:"risk_level"` // low, medium, high, critical
	RiskFactors datatypes.JSON `gorm:"type:jsonb" json:"risk_factors,omitempty"`
}

// TableName specifies the table name for ResourceStats
func (ResourceStats) TableName() string {
	return "resource_stats"
}

// BackupJob represents a backup job
type BackupJob struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	ResourceID      uint       `gorm:"not null;index" json:"resource_id"`
	Resource        Resource   `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"resource,omitempty"`
	JobType         string     `gorm:"not null;size:100" json:"job_type"` // full, incremental, differential
	Status          string     `gorm:"not null;size:50" json:"status"`    // pending, running, completed, failed
	BackupLocation  string     `gorm:"size:500" json:"backup_location"`
	BackupSizeBytes int64      `json:"backup_size_bytes"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ErrorMessage    string     `gorm:"type:text" json:"error_message,omitempty"`
	CreatedBy       uint       `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
}

// TableName specifies the table name for BackupJob
func (BackupJob) TableName() string {
	return "backup_jobs"
}

// ProvisioningJob represents a provisioning job
type ProvisioningJob struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	ResourceID   uint           `gorm:"not null;index" json:"resource_id"`
	Resource     Resource       `gorm:"foreignKey:ResourceID;constraint:OnDelete:CASCADE" json:"resource,omitempty"`
	JobType      string         `gorm:"not null;size:100" json:"job_type"` // provision, deprovision, scale, migrate
	Status       string         `gorm:"not null;size:50" json:"status"`    // pending, running, completed, failed, rolled_back
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
	Logs         datatypes.JSON `gorm:"type:jsonb" json:"logs,omitempty"`
	ErrorMessage string         `gorm:"type:text" json:"error_message,omitempty"`
	CreatedBy    uint           `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// TableName specifies the table name for ProvisioningJob
func (ProvisioningJob) TableName() string {
	return "provisioning_jobs"
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	UserID       *uint          `gorm:"index" json:"user_id,omitempty"`
	User         *User          `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
	Action       string         `gorm:"not null;size:100" json:"action"` // create, update, delete, login, logout, etc.
	ResourceType string         `gorm:"size:100" json:"resource_type"`
	ResourceID   *uint          `gorm:"index" json:"resource_id,omitempty"`
	TeamID       *uint          `gorm:"index" json:"team_id,omitempty"`
	Team         *Team          `gorm:"foreignKey:TeamID;constraint:OnDelete:SET NULL" json:"team,omitempty"`
	Details      datatypes.JSON `gorm:"type:jsonb" json:"details,omitempty"`
	IPAddress    string         `gorm:"size:45" json:"ip_address"`
	UserAgent    string         `gorm:"type:text" json:"user_agent,omitempty"`
	Timestamp    time.Time      `gorm:"index" json:"timestamp"`
}

// TableName specifies the table name for AuditLog
func (AuditLog) TableName() string {
	return "audit_logs"
}

// BeforeCreate hook for AuditLog to set timestamp
func (a *AuditLog) BeforeCreate(tx *gorm.DB) error {
	if a.Timestamp.IsZero() {
		a.Timestamp = time.Now().UTC()
	}
	return nil
}

// PolicyRule represents a security or governance policy
type PolicyRule struct {
	ID          string         `gorm:"primaryKey;size:100" json:"id"`
	Name        string         `gorm:"not null;size:255" json:"name"`
	Tenant      string         `gorm:"index;size:100" json:"tenant"`
	Labels      datatypes.JSON `gorm:"type:jsonb" json:"labels"`       // []string
	Action      string         `gorm:"not null;size:50" json:"action"` // allow, deny, redact, warn
	Scope       string         `gorm:"size:255" json:"scope"`
	Regions     datatypes.JSON `gorm:"type:jsonb" json:"regions"` // []string
	Description string         `gorm:"type:text" json:"description"`
	Priority    int            `gorm:"default:0" json:"priority"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// TableName specifies the table name for PolicyRule
func (PolicyRule) TableName() string {
	return "policy_rules"
}

// SCIMUser represents a SCIM-compliant user identity
type SCIMUser struct {
	ID           string         `gorm:"primaryKey;size:100" json:"id"`
	ExternalID   string         `gorm:"size:255" json:"externalId"`
	Tenant       string         `gorm:"index;size:100" json:"tenant"`
	UserName     string         `gorm:"index;not null;size:255" json:"userName"` // Removed uniqueIndex to allow same username in different tenants
	DisplayName  string         `gorm:"size:255" json:"displayName"`
	Emails       datatypes.JSON `gorm:"type:jsonb" json:"emails"` // []SCIMEmail
	Active       bool           `gorm:"default:true" json:"active"`
	Groups       datatypes.JSON `gorm:"type:jsonb" json:"groups"` // []SCIMGroupRef
	ResourceType string         `gorm:"default:User" json:"resourceType"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// TableName specifies the table name for SCIMUser
func (SCIMUser) TableName() string {
	return "scim_users"
}

// SCIMGroup represents a SCIM-compliant group
type SCIMGroup struct {
	ID           string         `gorm:"primaryKey;size:100" json:"id"`
	Tenant       string         `gorm:"index;size:100" json:"tenant"`
	DisplayName  string         `gorm:"not null;size:255" json:"displayName"`
	Members      datatypes.JSON `gorm:"type:jsonb" json:"members"` // []SCIMMember
	ResourceType string         `gorm:"default:Group" json:"resourceType"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// TableName specifies the table name for SCIMGroup
func (SCIMGroup) TableName() string {
	return "scim_groups"
}

// ========== Helper Methods ==========

// IsExpired returns whether a certificate is expired
func (c *Certificate) IsExpired() bool {
	return time.Now().UTC().After(c.ValidUntil)
}

// ExpiresIn returns the number of days until certificate expires
func (c *Certificate) ExpiresIn() int {
	days := int(time.Until(c.ValidUntil).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// NeedsRenewal returns whether a certificate needs renewal
func (c *Certificate) NeedsRenewal() bool {
	if !c.AutoRenew {
		return false
	}
	expiresIn := c.ExpiresIn()
	return expiresIn <= c.RenewalThresholdDays
}

// HasTeamRole returns whether a user has a specific role in a team
func (tm *TeamMembership) HasRole(role string) bool {
	return tm.Role == role
}

// IsAdmin returns whether a team member is an admin
func (tm *TeamMembership) IsAdmin() bool {
	return tm.HasRole("team_admin")
}

// IsMaintainer returns whether a team member is a maintainer
func (tm *TeamMembership) IsMaintainer() bool {
	return tm.HasRole("team_maintainer")
}

// IsViewer returns whether a team member is a viewer
func (tm *TeamMembership) IsViewer() bool {
	return tm.HasRole("team_viewer")
}

// IsResourceActionAllowed returns whether a resource action is allowed
func (r *Resource) IsResourceActionAllowed(action string) bool {
	switch action {
	case "backup":
		return r.CanBackup
	case "monitor":
		return r.CanMonitor
	case "scale":
		return r.CanScale
	case "migrate":
		return r.CanMigrate
	default:
		return false
	}
}

// HasValidCertificate returns whether a resource has a valid, non-expired certificate
func (r *Resource) HasValidCertificate() bool {
	if r.TLSCertID == nil {
		return false
	}
	if len(r.Certificates) == 0 {
		return false
	}
	for _, cert := range r.Certificates {
		if cert.ID == *r.TLSCertID && !cert.IsExpired() {
			return true
		}
	}
	return false
}

// GetLatestStats returns the most recent stats for a resource
func (r *Resource) GetLatestStats() *ResourceStats {
	if len(r.Stats) == 0 {
		return nil
	}
	latest := r.Stats[0]
	for i := 1; i < len(r.Stats); i++ {
		if r.Stats[i].Timestamp.After(latest.Timestamp) {
			latest = r.Stats[i]
		}
	}
	return &latest
}

// HasPendingProvisioningJobs returns whether a resource has pending provisioning jobs
func (r *Resource) HasPendingProvisioningJobs() bool {
	for _, job := range r.ProvisioningJobs {
		if job.Status == "pending" || job.Status == "running" {
			return true
		}
	}
	return false
}

// CanPerformAction returns whether an action can be performed on the resource
func (r *Resource) CanPerformAction() bool {
	if r.Status != "active" {
		return false
	}
	if r.HasPendingProvisioningJobs() {
		return false
	}
	return true
}
