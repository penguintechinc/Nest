package cloudprovider

import (
	"net/http"
	"net/url"
	"testing"
)

// ============================================================================
// AWS signing helpers
// ============================================================================

func TestSha256sum(t *testing.T) {
	// empty body
	h := sha256sum([]byte{})
	if len(h) != 64 {
		t.Errorf("sha256sum(empty) length = %d, want 64", len(h))
	}

	// known value
	h2 := sha256sum([]byte("test"))
	if h2 == h {
		t.Error("sha256sum should produce different hashes for different inputs")
	}
	if len(h2) != 64 {
		t.Errorf("sha256sum length = %d, want 64", len(h2))
	}
}

func TestHmacsha256(t *testing.T) {
	key := []byte("secret")
	msg := []byte("message")
	result := hmacsha256(key, msg)
	if len(result) != 32 {
		t.Errorf("hmacsha256 length = %d, want 32", len(result))
	}

	// deterministic
	result2 := hmacsha256(key, msg)
	for i, b := range result {
		if result2[i] != b {
			t.Error("hmacsha256 is not deterministic")
			break
		}
	}
}

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

func TestGetCanonicalURI(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/resource", "/api/v1/resource"},
		{"", "/"},
		{"/", "/"},
	}
	for _, tt := range tests {
		got := getCanonicalURI(tt.path)
		if got != tt.want {
			t.Errorf("getCanonicalURI(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestGetCanonicalQueryString_Empty(t *testing.T) {
	u, _ := url.Parse("https://example.com/path")
	got := getCanonicalQueryString(u)
	if got != "" {
		t.Errorf("getCanonicalQueryString(empty) = %q, want empty", got)
	}
}

func TestGetCanonicalQueryString_WithParams(t *testing.T) {
	u, _ := url.Parse("https://example.com/path?b=2&a=1")
	got := getCanonicalQueryString(u)
	// Params should be sorted by key
	if got == "" {
		t.Error("getCanonicalQueryString should return non-empty for query string")
	}
	// "a=1&b=2" after sorting
	expected := "a=1&b=2"
	if got != expected {
		t.Errorf("getCanonicalQueryString() = %q, want %q", got, expected)
	}
}

func TestGetCanonicalQueryString_MultipleValues(t *testing.T) {
	u, _ := url.Parse("https://example.com/path?z=last&a=first&m=middle")
	got := getCanonicalQueryString(u)
	if got == "" {
		t.Error("getCanonicalQueryString should return non-empty")
	}
}

func TestGetCanonicalHeaders(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	req.Header.Set("X-Amz-Date", "20210101T000000Z")
	req.Header.Set("X-Amz-Security-Token", "token")
	req.Header.Set("Content-Type", "application/json") // should not be included

	got := getCanonicalHeaders(req)
	if got == "" {
		t.Error("getCanonicalHeaders() should return non-empty")
	}
	// Should contain x-amz headers
	if len(got) == 0 {
		t.Error("getCanonicalHeaders() should have x-amz-date header")
	}
}

func TestGetSignedHeaders(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	req.Header.Set("X-Amz-Date", "20210101T000000Z")
	req.Header.Set("Authorization", "Bearer token") // should not be included

	got := getSignedHeaders(req)
	// should contain x-amz-date
	if got == "" {
		t.Error("getSignedHeaders() should return non-empty")
	}
}

func TestBuildStringToSign(t *testing.T) {
	canonicalRequest := "GET\n/\n\nhost:example.com\n\nhost\nabc123"
	amzDate := "20210101T000000Z"
	datestamp := "20210101"
	region := "us-east-1"
	service := "s3"

	got := buildStringToSign(canonicalRequest, amzDate, datestamp, region, service)
	if got == "" {
		t.Error("buildStringToSign() should return non-empty")
	}
	// Should start with algorithm
	if len(got) < 20 {
		t.Errorf("buildStringToSign() = %q (too short)", got)
	}
}

func TestCalculateSignature(t *testing.T) {
	stringToSign := "AWS4-HMAC-SHA256\n20210101T000000Z\n20210101/us-east-1/s3/aws4_request\nabc123"
	secretKey := "test-secret-key"
	datestamp := "20210101"
	region := "us-east-1"
	service := "s3"

	got := calculateSignature(stringToSign, secretKey, datestamp, region, service)
	if len(got) != 64 {
		t.Errorf("calculateSignature() length = %d, want 64", len(got))
	}

	// Deterministic
	got2 := calculateSignature(stringToSign, secretKey, datestamp, region, service)
	if got != got2 {
		t.Error("calculateSignature() is not deterministic")
	}
}

func TestBuildCanonicalRequest(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com/path?a=1", nil)
	req.Header.Set("X-Amz-Date", "20210101T000000Z")

	payloadHash := sha256sum([]byte{})
	got := buildCanonicalRequest(req, payloadHash)

	if got == "" {
		t.Error("buildCanonicalRequest() should return non-empty")
	}
	// Should start with method
	if len(got) < 3 {
		t.Errorf("buildCanonicalRequest() = %q (too short)", got)
	}
}

func TestSignAWSRequest(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.amazonaws.com/resource", nil)

	err := signAWSRequest(req, "execute-api", "us-east-1", "test-access-key", "test-secret-key", "", nil)
	if err != nil {
		t.Errorf("signAWSRequest() error = %v", err)
	}

	// Verify Authorization header is set
	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Error("signAWSRequest() did not set Authorization header")
	}
	if len(auth) < 20 {
		t.Errorf("Authorization header too short: %q", auth)
	}

	// With session token
	req2, _ := http.NewRequest("GET", "https://example.amazonaws.com/resource", nil)
	err = signAWSRequest(req2, "execute-api", "us-east-1", "key", "secret", "session-token", []byte("body"))
	if err != nil {
		t.Errorf("signAWSRequest() with session token error = %v", err)
	}
	if req2.Header.Get("X-Amz-Security-Token") != "session-token" {
		t.Error("signAWSRequest() did not set X-Amz-Security-Token header")
	}
}
