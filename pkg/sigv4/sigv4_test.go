package sigv4

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// pinClock fixes the signing timestamp for the duration of a test so that
// signatures are reproducible.
func pinClock(t *testing.T, ts time.Time) {
	t.Helper()
	orig := timeNow
	timeNow = func() time.Time { return ts }
	t.Cleanup(func() { timeNow = orig })
}

var fixedTime = time.Date(2026, 8, 21, 12, 36, 0, 0, time.UTC)

// TestSignIncludesHostHeader is the regression guard for the bug this package
// was extracted to fix. Go keeps Host outside req.Header, so the original
// implementation's `for k := range req.Header` loop never saw it and produced
// SignedHeaders=x-amz-content-sha256;x-amz-date. AWS rejects any signature
// whose SignedHeaders omits host, so every signed request would have failed
// with SignatureDoesNotMatch against real AWS or any S3-compatible provider.
func TestSignIncludesHostHeader(t *testing.T) {
	pinClock(t, fixedTime)

	req, err := http.NewRequest("GET", "https://rds.us-east-1.amazonaws.com/?Action=DescribeDBInstances", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if err := Sign(req, "rds", "us-east-1", "AKIATEST", "secret", "", nil); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	auth := req.Header.Get("Authorization")
	if !strings.Contains(auth, "SignedHeaders=") {
		t.Fatalf("no SignedHeaders in Authorization: %q", auth)
	}
	signed := auth[strings.Index(auth, "SignedHeaders=")+len("SignedHeaders="):]
	signed = signed[:strings.Index(signed, ",")]

	if !strings.Contains(signed, "host") {
		t.Errorf("SignedHeaders omits host (%q) — AWS would reject this signature", signed)
	}
	// Host must sort first among host;x-amz-*.
	if !strings.HasPrefix(signed, "host;") {
		t.Errorf("SignedHeaders not canonically ordered: %q", signed)
	}
}

// The host must be signed even when it comes from the URL rather than req.Host.
func TestSignHostFromURLWhenRequestHostEmpty(t *testing.T) {
	pinClock(t, fixedTime)

	req := &http.Request{
		Method: "GET",
		Header: http.Header{},
	}
	u, _ := url.Parse("https://nyc3.digitaloceanspaces.com/my-bucket")
	req.URL = u
	req.Host = "" // force the URL fallback

	if err := Sign(req, "s3", "nyc3", "key", "secret", "", nil); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !strings.Contains(req.Header.Get("Authorization"), "host") {
		t.Error("host not signed when req.Host is empty")
	}
	if got := requestHost(req); got != "nyc3.digitaloceanspaces.com" {
		t.Errorf("requestHost() = %q", got)
	}
}

func TestSignSetsRequiredHeaders(t *testing.T) {
	pinClock(t, fixedTime)

	req, _ := http.NewRequest("PUT", "https://nyc3.digitaloceanspaces.com/bucket", nil)
	if err := Sign(req, "s3", "nyc3", "AKIA", "secret", "", []byte("payload")); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if got := req.Header.Get("X-Amz-Date"); got != "20260821T123600Z" {
		t.Errorf("X-Amz-Date = %q", got)
	}
	if req.Header.Get("x-amz-content-sha256") == "" {
		t.Error("x-amz-content-sha256 not set")
	}
	// The content hash must be of the body, not of the empty string.
	if req.Header.Get("x-amz-content-sha256") == sha256sum([]byte{}) {
		t.Error("content hash is of empty body despite a payload being supplied")
	}
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIA/20260821/nyc3/s3/aws4_request") {
		t.Errorf("unexpected credential scope: %q", auth)
	}
}

func TestSignSessionToken(t *testing.T) {
	pinClock(t, fixedTime)

	req, _ := http.NewRequest("GET", "https://example.amazonaws.com/r", nil)
	if err := Sign(req, "execute-api", "us-east-1", "key", "secret", "session-token", nil); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if req.Header.Get("X-Amz-Security-Token") != "session-token" {
		t.Error("X-Amz-Security-Token not set")
	}
	// A session token participates in the signature, so it must be signed.
	if !strings.Contains(req.Header.Get("Authorization"), "x-amz-security-token") {
		t.Error("x-amz-security-token not included in SignedHeaders")
	}
}

func TestSignDeterministicUnderFixedClock(t *testing.T) {
	pinClock(t, fixedTime)

	sign := func() string {
		req, _ := http.NewRequest("GET", "https://s3.amazonaws.com/b?x=1", nil)
		_ = Sign(req, "s3", "us-east-1", "key", "secret", "", nil)
		return req.Header.Get("Authorization")
	}
	if a, b := sign(), sign(); a != b {
		t.Errorf("signature not deterministic:\n%s\n%s", a, b)
	}
}

// Any change to a signing input must change the signature. This is what makes
// the signature meaningful; a signer that ignored an input would still pass
// "returns non-empty" assertions.
func TestSignatureRespondsToEveryInput(t *testing.T) {
	pinClock(t, fixedTime)

	base := func(mut func(*http.Request), service, region, key, secret string) string {
		req, _ := http.NewRequest("GET", "https://s3.amazonaws.com/bucket?a=1", nil)
		if mut != nil {
			mut(req)
		}
		_ = Sign(req, service, region, key, secret, "", nil)
		return req.Header.Get("Authorization")
	}

	ref := base(nil, "s3", "us-east-1", "key", "secret")

	cases := map[string]string{
		"different secret":     base(nil, "s3", "us-east-1", "key", "secret2"),
		"different region":     base(nil, "s3", "eu-west-1", "key", "secret"),
		"different service":    base(nil, "ec2", "us-east-1", "key", "secret"),
		"different access key": base(nil, "s3", "us-east-1", "key2", "secret"),
		"different host": func() string {
			req, _ := http.NewRequest("GET", "https://other.example.com/bucket?a=1", nil)
			_ = Sign(req, "s3", "us-east-1", "key", "secret", "", nil)
			return req.Header.Get("Authorization")
		}(),
		"different path": func() string {
			req, _ := http.NewRequest("GET", "https://s3.amazonaws.com/other?a=1", nil)
			_ = Sign(req, "s3", "us-east-1", "key", "secret", "", nil)
			return req.Header.Get("Authorization")
		}(),
		"different query": func() string {
			req, _ := http.NewRequest("GET", "https://s3.amazonaws.com/bucket?a=2", nil)
			_ = Sign(req, "s3", "us-east-1", "key", "secret", "", nil)
			return req.Header.Get("Authorization")
		}(),
	}
	for name, got := range cases {
		if got == ref {
			t.Errorf("%s produced an identical signature — that input is not being signed", name)
		}
	}
}

func TestSignRejectsMissingCredentials(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://s3.amazonaws.com/b", nil)
	if err := Sign(req, "s3", "us-east-1", "", "secret", "", nil); err == nil {
		t.Error("expected error for empty access key")
	}
	if err := Sign(req, "s3", "us-east-1", "key", "", "", nil); err == nil {
		t.Error("expected error for empty secret key")
	}
}

func TestSignRejectsNilRequest(t *testing.T) {
	if err := Sign(nil, "s3", "us-east-1", "key", "secret", "", nil); err == nil {
		t.Error("expected error for nil request")
	}
}

func TestSha256sum(t *testing.T) {
	// Known SHA-256 of the empty string.
	if got := sha256sum([]byte{}); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("sha256sum(empty) = %q", got)
	}
	if len(sha256sum([]byte("test"))) != 64 {
		t.Error("sha256sum should return 64 hex chars")
	}
}

func TestHmacsha256(t *testing.T) {
	key, msg := []byte("key"), []byte("msg")
	a, b := hmacsha256(key, msg), hmacsha256(key, msg)
	if string(a) != string(b) {
		t.Error("hmacsha256 is not deterministic")
	}
	if len(a) != 32 {
		t.Errorf("hmacsha256 length = %d, want 32", len(a))
	}
	if string(hmacsha256([]byte("other"), msg)) == string(a) {
		t.Error("different keys produced the same HMAC")
	}
}

func TestGetCanonicalURI(t *testing.T) {
	cases := map[string]string{"": "/", "/": "/", "/path": "/path"}
	for in, want := range cases {
		if got := getCanonicalURI(in); got != want {
			t.Errorf("getCanonicalURI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGetCanonicalQueryString(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"https://e.com", ""},
		{"https://e.com?b=2&a=1", "a=1&b=2"}, // sorted by key
		{"https://e.com?a=1", "a=1"},
	}
	for _, tc := range cases {
		u, _ := url.Parse(tc.raw)
		if got := getCanonicalQueryString(u); got != tc.want {
			t.Errorf("getCanonicalQueryString(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestGetCanonicalHeadersIncludesHostAndAmzOnly(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	req.Header.Set("X-Amz-Date", "20210101T000000Z")
	req.Header.Set("Content-Type", "application/json") // must NOT be signed

	got := getCanonicalHeaders(req)
	if !strings.Contains(got, "host:example.com\n") {
		t.Errorf("canonical headers missing host: %q", got)
	}
	if !strings.Contains(got, "x-amz-date:20210101T000000Z\n") {
		t.Errorf("canonical headers missing x-amz-date: %q", got)
	}
	if strings.Contains(got, "content-type") {
		t.Errorf("content-type must not be signed: %q", got)
	}
}

func TestGetSignedHeadersSortedAndScoped(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	req.Header.Set("X-Amz-Date", "20210101T000000Z")
	req.Header.Set("Authorization", "Bearer token") // must NOT be signed

	got := getSignedHeaders(req)
	if got != "host;x-amz-date" {
		t.Errorf("getSignedHeaders() = %q, want %q", got, "host;x-amz-date")
	}
}

func TestBuildStringToSignStructure(t *testing.T) {
	got := buildStringToSign("canonical", "20210101T000000Z", "20210101", "us-east-1", "s3")
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("string to sign should have 4 lines, got %d: %q", len(lines), got)
	}
	if lines[0] != "AWS4-HMAC-SHA256" {
		t.Errorf("line 0 = %q", lines[0])
	}
	if lines[1] != "20210101T000000Z" {
		t.Errorf("line 1 = %q", lines[1])
	}
	if lines[2] != "20210101/us-east-1/s3/aws4_request" {
		t.Errorf("line 2 = %q", lines[2])
	}
	if lines[3] != sha256sum([]byte("canonical")) {
		t.Errorf("line 3 is not the canonical request hash")
	}
}

func TestCalculateSignature(t *testing.T) {
	sts := "AWS4-HMAC-SHA256\n20210101T000000Z\n20210101/us-east-1/s3/aws4_request\nabc123"
	got := calculateSignature(sts, "secret", "20210101", "us-east-1", "s3")
	if len(got) != 64 {
		t.Errorf("signature length = %d, want 64", len(got))
	}
	if got != calculateSignature(sts, "secret", "20210101", "us-east-1", "s3") {
		t.Error("calculateSignature is not deterministic")
	}
	if got == calculateSignature(sts, "secret", "20210102", "us-east-1", "s3") {
		t.Error("datestamp is not part of the derived signing key")
	}
}

func TestBuildCanonicalRequestStructure(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com/path?a=1", nil)
	req.Header.Set("X-Amz-Date", "20210101T000000Z")

	got := buildCanonicalRequest(req, sha256sum([]byte{}))
	lines := strings.Split(got, "\n")
	if lines[0] != "GET" {
		t.Errorf("method line = %q", lines[0])
	}
	if lines[1] != "/path" {
		t.Errorf("uri line = %q", lines[1])
	}
	if lines[2] != "a=1" {
		t.Errorf("query line = %q", lines[2])
	}
	if !strings.Contains(got, "host:example.com") {
		t.Error("canonical request omits host")
	}
}
