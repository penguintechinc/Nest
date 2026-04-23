package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type Schema struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SchemaSpec   `json:"spec,omitempty"`
	Status            SchemaStatus `json:"status,omitempty"`
}

type SchemaSpec struct {
	// Resource references the parent DataResource
	Resource string `json:"resource"`
	// Dialect: nest or passthrough
	// +kubebuilder:validation:Enum=nest;passthrough
	// +kubebuilder:default=nest
	Dialect string `json:"dialect,omitempty"`
	// Version is the monotonic schema version (server-assigned on apply)
	Version int64 `json:"version,omitempty"`
	// Enforcement mode
	// +kubebuilder:validation:Enum=strict;advisory;off
	// +kubebuilder:default=advisory
	Enforcement string `json:"enforcement,omitempty"`
	// ValidateWrites enables write-time schema enforcement at DBLB
	ValidateWrites bool `json:"validateWrites,omitempty"`
	// Fields defines the engine-agnostic schema fields
	Fields []SchemaField `json:"fields,omitempty"`
	// Indexes defines secondary indexes
	Indexes []SchemaIndex `json:"indexes,omitempty"`
}

type SchemaField struct {
	// Name is the field/column name
	Name string `json:"name"`
	// Type is the Nest engine-agnostic type
	Type string `json:"type"`
	// Required marks the field as NOT NULL
	Required bool `json:"required,omitempty"`
	// PrimaryKey marks this as the primary key
	PrimaryKey bool `json:"primaryKey,omitempty"`
	// Default value expression
	Default string `json:"default,omitempty"`
	// Constraints for validation
	Constraints []FieldConstraint `json:"constraints,omitempty"`
	// Labels for Indexer classification (e.g., pii.email)
	Labels []string `json:"labels,omitempty"`
}

type FieldConstraint struct {
	// Type: gte, lte, maxLen, format, enum, etc.
	Type string `json:"type"`
	// Value is the constraint value as string
	Value string `json:"value,omitempty"`
}

type SchemaIndex struct {
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
	Unique bool     `json:"unique,omitempty"`
}

type SchemaStatus struct {
	// AppliedVersion is the currently applied version
	AppliedVersion int64 `json:"appliedVersion,omitempty"`
	// TargetVersion is the desired version
	TargetVersion int64 `json:"targetVersion,omitempty"`
	// Drift indicates schema drift
	// +kubebuilder:validation:Enum=none;detected
	Drift      string             `json:"drift,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type SchemaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Schema `json:"items"`
}
