package protocol

import (
	"bytes"
	"crypto/sha1"
	"testing"
)

// mysqlPacket frames a payload as a MySQL wire packet: 3-byte LE length + seq + payload.
func mysqlPacket(seq byte, payload []byte) []byte {
	n := len(payload)
	return append([]byte{byte(n), byte(n >> 8), byte(n >> 16), seq}, payload...)
}

func TestCalculateNativePasswordAuth(t *testing.T) {
	ah := NewMySQLAuthHandler("secret")
	salt1 := []byte("12345678")             // 8 bytes
	salt2 := []byte("123456789012")         // 12 bytes -> 20 total
	got := ah.calculateNativePasswordAuth("secret", salt1, salt2)
	if len(got) != 20 {
		t.Fatalf("auth response length = %d, want 20", len(got))
	}

	// Recompute independently: SHA1(SHA1(pw)+salt) XOR SHA1(pw).
	pw := sha1.Sum([]byte("secret"))
	h := sha1.New()
	h.Write(pw[:])
	h.Write(append(append([]byte{}, salt1...), salt2...))
	stage := h.Sum(nil)
	want := make([]byte, 20)
	for i := range want {
		want[i] = stage[i] ^ pw[i]
	}
	if !bytes.Equal(got, want) {
		t.Errorf("native password auth mismatch")
	}
}

func TestMySQLResponseAccumulator_OK(t *testing.T) {
	acc := NewMySQLResponseAccumulator()
	// OK packet: payload begins 0x00.
	done, err := acc.AddPacket(mysqlPacket(1, []byte{0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}))
	if err != nil || !done {
		t.Fatalf("OK packet: done=%v err=%v, want done=true", done, err)
	}
	if !acc.IsComplete() {
		t.Error("IsComplete should be true after OK")
	}
	if len(acc.GetPackets()) != 1 {
		t.Errorf("GetPackets len = %d, want 1", len(acc.GetPackets()))
	}
	if len(acc.GetAllBytes()) == 0 {
		t.Error("GetAllBytes should be non-empty")
	}
}

func TestMySQLResponseAccumulator_Error(t *testing.T) {
	acc := NewMySQLResponseAccumulator()
	// ERR packet: payload begins 0xFF.
	done, err := acc.AddPacket(mysqlPacket(1, []byte{0xFF, 0x15, 0x04, 'e', 'r', 'r'}))
	if err != nil || !done {
		t.Fatalf("ERR packet: done=%v err=%v, want done=true", done, err)
	}
}

func TestMySQLResponseAccumulator_ShortPacket(t *testing.T) {
	acc := NewMySQLResponseAccumulator()
	if _, err := acc.AddPacket([]byte{0x01, 0x02}); err == nil {
		t.Error("expected error for too-short packet")
	}
}
