package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// pgMsg frames a typed PostgreSQL backend message: type + int32 length + payload.
func pgMsg(typ byte, payload []byte) []byte {
	b := []byte{typ}
	b = binary.BigEndian.AppendUint32(b, uint32(len(payload)+4))
	return append(b, payload...)
}

// authPayload builds an Authentication message payload (auth-type int32 + extra).
func authPayload(authType uint32, extra []byte) []byte {
	return append(binary.BigEndian.AppendUint32(nil, authType), extra...)
}

func TestHandleAuthMessage_Branches(t *testing.T) {
	// AuthenticationOk (0) -> nil, no write.
	ah := NewAuthHandler("u", "pw")
	if err := ah.handleAuthMessage(&bytes.Buffer{}, authPayload(BackendAuthOk, nil)); err != nil {
		t.Errorf("AuthOk: %v", err)
	}

	// Cleartext (3) -> writes a PasswordMessage.
	var clear bytes.Buffer
	if err := ah.handleAuthMessage(&clear, authPayload(BackendAuthCleartextPw, nil)); err != nil {
		t.Fatalf("cleartext: %v", err)
	}
	if clear.Len() == 0 || clear.Bytes()[0] != 'p' {
		t.Error("cleartext should write a 'p' PasswordMessage")
	}

	// Cleartext with no password -> error.
	ahNoPw := NewAuthHandler("u", "")
	if err := ahNoPw.handleAuthMessage(&bytes.Buffer{}, authPayload(BackendAuthCleartextPw, nil)); err == nil {
		t.Error("cleartext with empty password should error")
	}

	// MD5 (5) with 4-byte salt -> writes md5 PasswordMessage.
	var md5buf bytes.Buffer
	if err := ah.handleAuthMessage(&md5buf, authPayload(BackendAuthMD5Pw, []byte{1, 2, 3, 4})); err != nil {
		t.Fatalf("md5: %v", err)
	}
	if md5buf.Len() == 0 || md5buf.Bytes()[0] != 'p' {
		t.Error("md5 should write a 'p' PasswordMessage")
	}

	// Unsupported auth type -> error.
	if err := ah.handleAuthMessage(&bytes.Buffer{}, authPayload(BackendAuthGSS, nil)); err == nil {
		t.Error("unsupported auth type should error")
	}
}

func TestHandleStartup_MessageTypes(t *testing.T) {
	// ParameterStatus (S) + BackendKeyData (K) + ReadyForQuery (Z) -> success.
	var ok bytes.Buffer
	ok.Write(pgMsg('S', []byte("server_version\x0016.0\x00")))
	ok.Write(pgMsg('K', make([]byte, 8)))
	ok.Write(pgMsg('Z', []byte{'I'}))
	if err := NewAuthHandler("u", "p").HandleStartup(&ok); err != nil {
		t.Fatalf("HandleStartup(S,K,Z) = %v, want nil", err)
	}

	// ErrorResponse (E) -> error.
	var bad bytes.Buffer
	bad.Write(pgMsg('E', append([]byte{'M'}, append([]byte("nope"), 0, 0)...)))
	if err := NewAuthHandler("u", "p").HandleStartup(&bad); err == nil {
		t.Error("HandleStartup with ErrorResponse should error")
	}

	// Unexpected message type -> error.
	var weird bytes.Buffer
	weird.Write(pgMsg('X', []byte{0x01}))
	if err := NewAuthHandler("u", "p").HandleStartup(&weird); err == nil {
		t.Error("HandleStartup with unexpected type should error")
	}
}
