package cloudprovider

import (
	"testing"
)

// ============================================================================
// AWS ARN/identifier helpers
//
// The SigV4 signing helpers formerly tested here moved to pkg/sigv4 along with
// the implementation.
// ============================================================================

func TestExtractDBInstanceID(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{"arn:aws:rds:us-east-1:123:db:mydb", "mydb"},
		{"arn:aws:rds:us-east-1:123:db:another-db", "another-db"},
		{"single", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractDBInstanceID(tt.arn)
		if got != tt.want {
			t.Errorf("extractDBInstanceID(%q) = %q, want %q", tt.arn, got, tt.want)
		}
	}
}

func TestExtractClusterID(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{"arn:aws:elasticache:us-east-1:123:cluster:my-cluster", "my-cluster"},
		{"single", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractClusterID(tt.arn)
		if got != tt.want {
			t.Errorf("extractClusterID(%q) = %q, want %q", tt.arn, got, tt.want)
		}
	}
}
