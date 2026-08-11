package protocol

import (
	"encoding/base64"
	"testing"
)

func TestParseSCRAMServerFirst(t *testing.T) {
	// Valid message.
	n, s, i, err := parseSCRAMServerFirst("r=abc123,s=c2FsdA==,i=4096")
	if err != nil || n != "abc123" || s != "c2FsdA==" || i != "4096" {
		t.Errorf("valid = (%q,%q,%q,%v)", n, s, i, err)
	}

	// 'e=' error attribute.
	if _, _, _, err := parseSCRAMServerFirst("e=other-error"); err == nil {
		t.Error("expected error for e= attribute")
	}

	// Missing fields (and a too-short attr that is skipped).
	if _, _, _, err := parseSCRAMServerFirst("r=abc,x"); err == nil {
		t.Error("expected malformed error when salt/iter missing")
	}
}

func TestHandleSASLFinal(t *testing.T) {
	sig := []byte{0x01, 0x02, 0x03, 0x04}
	v := base64.StdEncoding.EncodeToString(sig)

	// No stored signature -> error.
	if err := (&AuthHandler{}).handleSASLFinal([]byte("v=" + v)); err == nil {
		t.Error("expected error when server signature not yet computed")
	}

	ah := &AuthHandler{scramServerSignature: sig}

	// Matching verifier -> success.
	if err := ah.handleSASLFinal([]byte("v=" + v)); err != nil {
		t.Errorf("matching verifier: %v", err)
	}

	// Mismatched verifier.
	other := base64.StdEncoding.EncodeToString([]byte{0x09, 0x09})
	if err := ah.handleSASLFinal([]byte("v=" + other)); err == nil {
		t.Error("expected signature-mismatch error")
	}

	// Invalid base64.
	if err := ah.handleSASLFinal([]byte("v=!!!not-base64")); err == nil {
		t.Error("expected base64 decode error")
	}

	// Server-reported error attribute.
	if err := ah.handleSASLFinal([]byte("e=invalid-proof")); err == nil {
		t.Error("expected SCRAM auth-failed error")
	}

	// Missing verifier entirely.
	if err := ah.handleSASLFinal([]byte("x=nope")); err == nil {
		t.Error("expected missing-verifier error")
	}
}
