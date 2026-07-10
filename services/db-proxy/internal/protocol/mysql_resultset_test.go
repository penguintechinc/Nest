package protocol

import "testing"

// TestResultSetCompletion drives the multi-packet result-set path:
// column-count -> column-def -> EOF, which the accumulator treats as complete.
func TestResultSetCompletion(t *testing.T) {
	acc := NewMySQLResponseAccumulator()

	// 1. Column count = 1 (length-encoded int < 0xFB) -> default branch, not done.
	if done, err := acc.AddPacket(mysqlPacket(1, []byte{0x01})); err != nil || done {
		t.Fatalf("column-count: done=%v err=%v, want done=false", done, err)
	}
	// 2. One column definition -> still not complete.
	if done, _ := acc.AddPacket(mysqlPacket(2, []byte{0x03, 'd', 'e', 'f'})); done {
		t.Error("column-def should not complete the result set")
	}
	// 3. Short EOF packet (0xFE) -> complete.
	if done, err := acc.AddPacket(mysqlPacket(3, []byte{0xFE, 0x00, 0x00, 0x02, 0x00})); err != nil || !done {
		t.Fatalf("EOF: done=%v err=%v, want done=true", done, err)
	}

	cols, err := acc.ParseResultSetMetadata()
	if err != nil || cols != 1 {
		t.Errorf("ParseResultSetMetadata = (%d, %v), want (1, nil)", cols, err)
	}
}

func TestParseResultSetMetadata_Types(t *testing.T) {
	cases := []struct {
		name    string
		first   byte
		wantCol int
		wantErr bool
	}{
		{"ok", 0x00, 0, false},
		{"err", 0xFF, 0, true},
		{"eof", 0xFE, 0, false},
	}
	for _, c := range cases {
		acc := NewMySQLResponseAccumulator()
		_, _ = acc.AddPacket(mysqlPacket(1, []byte{c.first, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}))
		cols, err := acc.ParseResultSetMetadata()
		if (err != nil) != c.wantErr || cols != c.wantCol {
			t.Errorf("%s: got (%d, %v), want (%d, err=%v)", c.name, cols, err, c.wantCol, c.wantErr)
		}
	}

	// No packets -> error.
	if _, err := NewMySQLResponseAccumulator().ParseResultSetMetadata(); err == nil {
		t.Error("empty accumulator should error")
	}
}

func TestDecodeLengthEncodedInt_Table(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		wantVal int
		wantN   int
	}{
		{"empty", nil, 0, 0},
		{"1-byte", []byte{0x05}, 5, 1},
		{"null", []byte{0xFB}, -1, 1},
		{"2-byte", []byte{0xFC, 0x01, 0x01}, 257, 3},
		{"2-byte-short", []byte{0xFC, 0x01}, 0, 0},
		{"3-byte", []byte{0xFD, 0x01, 0x00, 0x00}, 1, 4},
		{"3-byte-short", []byte{0xFD, 0x01}, 0, 0},
		{"8-byte", []byte{0xFE, 0x02, 0, 0, 0, 0, 0, 0, 0}, 2, 9},
		{"8-byte-short", []byte{0xFE, 0x02}, 0, 0},
	}
	for _, c := range cases {
		val, n := decodeLengthEncodedInt(c.data)
		if val != c.wantVal || n != c.wantN {
			t.Errorf("%s: decode = (%d, %d), want (%d, %d)", c.name, val, n, c.wantVal, c.wantN)
		}
	}
}
