package protocol

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func TestStartupMessage(t *testing.T) {
	msg := StartupMessage("alice", "appdb")
	if len(msg) < 8 {
		t.Fatalf("startup message too short: %d", len(msg))
	}
	// Length prefix must equal total length.
	if got := binary.BigEndian.Uint32(msg[:4]); int(got) != len(msg) {
		t.Errorf("length prefix = %d, want %d", got, len(msg))
	}
	// Protocol version 3.0.
	if got := binary.BigEndian.Uint32(msg[4:8]); got != (3 << 16) {
		t.Errorf("protocol version = %d, want %d", got, 3<<16)
	}
	body := string(msg[8:])
	for _, want := range []string{"user", "alice", "database", "appdb"} {
		if !strings.Contains(body, want) {
			t.Errorf("startup body missing %q", want)
		}
	}
}

func TestPasswordMessage(t *testing.T) {
	ah := NewAuthHandler("u", "p")
	msg := ah.passwordMessage("secret")
	if msg[0] != 'p' {
		t.Errorf("password message tag = %c, want p", msg[0])
	}
	if got := binary.BigEndian.Uint32(msg[1:5]); int(got) != len(msg)-1 {
		t.Errorf("length = %d, want %d", got, len(msg)-1)
	}
	if body := msg[5:]; string(body) != "secret\x00" {
		t.Errorf("body = %q, want secret\\x00", body)
	}
}

func TestMD5Password(t *testing.T) {
	ah := NewAuthHandler("alice", "pencil")
	salt := []byte{0x01, 0x02, 0x03, 0x04}
	got := ah.md5Password("alice", "pencil", salt)

	inner := md5.Sum([]byte("pencil" + "alice"))
	outer := md5.Sum(append([]byte(fmt.Sprintf("%x", inner)), salt...))
	want := "md5" + fmt.Sprintf("%x", outer)
	if got != want {
		t.Errorf("md5Password = %s, want %s", got, want)
	}
}

func TestParseErrorResponse(t *testing.T) {
	// Field 'M' (message) + 'S' (severity), terminated by a 0 field byte.
	var payload []byte
	payload = append(payload, 'S')
	payload = append(payload, []byte("FATAL")...)
	payload = append(payload, 0)
	payload = append(payload, 'M')
	payload = append(payload, []byte("password authentication failed")...)
	payload = append(payload, 0)
	payload = append(payload, 0) // end of fields

	got := parseErrorResponse(payload)
	if got != "password authentication failed" {
		t.Errorf("parseErrorResponse = %q, want the 'M' field", got)
	}
}
