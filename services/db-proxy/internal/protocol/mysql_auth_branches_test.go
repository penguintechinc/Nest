package protocol

import (
	"bytes"
	"testing"
)

func TestReadAuthResult_Branches(t *testing.T) {
	// OK packet.
	if err := NewMySQLAuthHandler("").ReadAuthResult(bytes.NewReader(mysqlPacket(2, []byte{0x00}))); err != nil {
		t.Errorf("OK: %v", err)
	}
	// ERR packet with message.
	if err := NewMySQLAuthHandler("").ReadAuthResult(bytes.NewReader(mysqlPacket(2, []byte{0xFF, 'b', 'a', 'd'}))); err == nil {
		t.Error("expected ERR auth error")
	}
	// ERR packet, single byte -> "unknown".
	if err := NewMySQLAuthHandler("").ReadAuthResult(bytes.NewReader(mysqlPacket(2, []byte{0xFF}))); err == nil {
		t.Error("expected ERR unknown error")
	}
	// Empty payload.
	if err := NewMySQLAuthHandler("").ReadAuthResult(bytes.NewReader(mysqlPacket(2, []byte{}))); err == nil {
		t.Error("expected empty-payload error")
	}
	// Unexpected packet type.
	if err := NewMySQLAuthHandler("").ReadAuthResult(bytes.NewReader(mysqlPacket(2, []byte{0x07}))); err == nil {
		t.Error("expected unexpected-type error")
	}
	// Truncated length header.
	if err := NewMySQLAuthHandler("").ReadAuthResult(bytes.NewReader([]byte{0x01})); err == nil {
		t.Error("expected length read error")
	}
	// Header ok but missing sequence byte.
	if err := NewMySQLAuthHandler("").ReadAuthResult(bytes.NewReader([]byte{0x01, 0x00, 0x00})); err == nil {
		t.Error("expected sequence read error")
	}
	// Header+seq ok but payload short.
	if err := NewMySQLAuthHandler("").ReadAuthResult(bytes.NewReader([]byte{0x05, 0x00, 0x00, 0x02, 0x00})); err == nil {
		t.Error("expected payload read error")
	}
}

func TestAddPacket_Branches(t *testing.T) {
	// Incomplete packet: length field exceeds actual payload.
	acc := NewMySQLResponseAccumulator()
	if _, err := acc.AddPacket([]byte{0x0A, 0x00, 0x00, 0x01, 0xAA, 0xBB}); err == nil {
		t.Error("expected incomplete-packet error")
	}

	// Empty first packet (zero-length payload).
	acc2 := NewMySQLResponseAccumulator()
	if _, err := acc2.AddPacket([]byte{0x00, 0x00, 0x00, 0x01}); err == nil {
		t.Error("expected empty-first-packet error")
	}

	// LOCAL INFILE (0xFB) single packet -> complete.
	acc3 := NewMySQLResponseAccumulator()
	if done, err := acc3.AddPacket(mysqlPacket(1, []byte{0xFB, 'f', 'i', 'l', 'e'})); err != nil || !done {
		t.Errorf("LOCAL INFILE: done=%v err=%v", done, err)
	}

	// Result set: column-count, a long data row (>10 bytes, not terminal), then short OK -> complete.
	acc4 := NewMySQLResponseAccumulator()
	if done, _ := acc4.AddPacket(mysqlPacket(1, []byte{0x01})); done {
		t.Error("column-count should not complete")
	}
	longRow := make([]byte, 12)
	longRow[0] = 0x05
	if done, _ := acc4.AddPacket(mysqlPacket(2, longRow)); done {
		t.Error("long data row should not complete")
	}
	if done, _ := acc4.AddPacket(mysqlPacket(3, []byte{0x00, 0x00, 0x00, 0x02, 0x00})); !done {
		t.Error("short OK should complete the result set")
	}
}

func TestParseResultSetMetadata_Branches(t *testing.T) {
	// First packet too short (4 bytes -> header only, no payload).
	shortAcc := NewMySQLResponseAccumulator()
	shortAcc.packets = [][]byte{{0x00, 0x00, 0x00, 0x01}}
	if _, err := shortAcc.ParseResultSetMetadata(); err == nil {
		t.Error("expected first-packet-too-short error")
	}

	// Decode failure: 0xFC marker but truncated -> n==0.
	badAcc := NewMySQLResponseAccumulator()
	badAcc.packets = [][]byte{{0x02, 0x00, 0x00, 0x01, 0xFC, 0x01}}
	if _, err := badAcc.ParseResultSetMetadata(); err == nil {
		t.Error("expected column-count decode error")
	}

	// Valid column count.
	okAcc := NewMySQLResponseAccumulator()
	if _, err := okAcc.AddPacket(mysqlPacket(1, []byte{0x03})); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if cols, err := okAcc.ParseResultSetMetadata(); err != nil || cols != 3 {
		t.Errorf("ParseResultSetMetadata = (%d, %v), want (3, nil)", cols, err)
	}
}
