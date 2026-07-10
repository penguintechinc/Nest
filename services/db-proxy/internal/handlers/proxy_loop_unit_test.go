package handlers

import (
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"go.uber.org/zap"
)

// chunkRW feeds ReadFrame data in caller-defined chunks (to exercise the
// "need more data" buffering loop) and captures writes. A zero-length chunk
// followed by more chunks is not produced; readErr (if set) is returned once
// all chunks are drained instead of io.EOF.
type chunkRW struct {
	chunks  [][]byte
	idx     int
	written []byte
	readErr error // returned after chunks exhausted (nil -> io.EOF)
	writeN  int   // if >0, Write reports this many bytes (partial write)
	writeE  error // if set, Write returns this error
}

func (c *chunkRW) Read(p []byte) (int, error) {
	if c.idx >= len(c.chunks) {
		if c.readErr != nil {
			return 0, c.readErr
		}
		return 0, io.EOF
	}
	n := copy(p, c.chunks[c.idx])
	c.idx++
	return n, nil
}

func (c *chunkRW) Write(p []byte) (int, error) {
	if c.writeE != nil {
		return 0, c.writeE
	}
	c.written = append(c.written, p...)
	if c.writeN > 0 {
		return c.writeN, nil // simulate partial write
	}
	return len(p), nil
}

// mysqlPacket frames a payload as a MySQL wire packet: 3-byte LE length + seq + payload.
func mysqlPacket(seq byte, payload []byte) []byte {
	n := len(payload)
	return append([]byte{byte(n), byte(n >> 8), byte(n >> 16), seq}, payload...)
}

func pgFrame(typ byte, payload []byte) []byte {
	f := make([]byte, 5+len(payload))
	f[0] = typ
	binary.BigEndian.PutUint32(f[1:5], uint32(4+len(payload)))
	copy(f[5:], payload)
	return f
}

// --- Framing ---

func TestPostgreSQLFramer_ReadFrame_Chunked(t *testing.T) {
	full := pgFrame('Q', []byte("SELECT 1"))
	// Deliver header and body across two Reads -> exercises buffering loop.
	rw := &chunkRW{chunks: [][]byte{full[:3], full[3:]}}
	fr := NewPostgreSQLFramer(rw)
	got, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if string(got) != string(full) {
		t.Errorf("frame mismatch: got %v want %v", got, full)
	}
	// Next read drains the buffer -> EOF.
	if _, err := fr.ReadFrame(); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestPostgreSQLFramer_ReadFrame_ReadError(t *testing.T) {
	fr := NewPostgreSQLFramer(&chunkRW{readErr: errors.New("boom")})
	if _, err := fr.ReadFrame(); err == nil {
		t.Error("expected read error")
	}
}

func TestMySQLFramer_ReadFrame_Chunked(t *testing.T) {
	full := mysqlPacket(1, []byte("hello"))
	rw := &chunkRW{chunks: [][]byte{full[:2], full[2:]}}
	fr := NewMySQLFramer(rw)
	got, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if string(got) != string(full) {
		t.Errorf("frame mismatch: got %v want %v", got, full)
	}
}

func TestMySQLFramer_ReadFrame_ReadError(t *testing.T) {
	fr := NewMySQLFramer(&chunkRW{readErr: errors.New("boom")})
	if _, err := fr.ReadFrame(); err == nil {
		t.Error("expected read error")
	}
}

func TestFramer_WriteFrame_Errors(t *testing.T) {
	// Both framers: hard write error propagates.
	if err := NewPostgreSQLFramer(&chunkRW{writeE: errors.New("nope")}).WriteFrame([]byte{0x01, 0x02}); err == nil {
		t.Error("PG: expected write error")
	}
	if err := NewMySQLFramer(&chunkRW{writeE: errors.New("nope")}).WriteFrame([]byte{0x01, 0x02}); err == nil {
		t.Error("MySQL: expected write error")
	}
	// Both framers: partial write -> "partial write" error.
	if err := NewPostgreSQLFramer(&chunkRW{writeN: 1}).WriteFrame([]byte{0x01, 0x02, 0x03}); err == nil {
		t.Error("PG: expected partial-write error")
	}
	if err := NewMySQLFramer(&chunkRW{writeN: 1}).WriteFrame([]byte{0x01, 0x02, 0x03}); err == nil {
		t.Error("MySQL: expected partial-write error")
	}
	// Happy path.
	ok := &chunkRW{}
	if err := NewPostgreSQLFramer(ok).WriteFrame([]byte{0x09}); err != nil {
		t.Errorf("WriteFrame ok: %v", err)
	}
	if len(ok.written) != 1 {
		t.Errorf("written = %d bytes, want 1", len(ok.written))
	}
}

func splitBytes(b []byte, size int) [][]byte {
	var out [][]byte
	for len(b) > 0 {
		n := size
		if n > len(b) {
			n = len(b)
		}
		out = append(out, b[:n])
		b = b[n:]
	}
	return out
}

func TestPostgreSQLFramer_ReadFrame_BufferGrow(t *testing.T) {
	// Frame larger than the initial 64KiB buffer forces the grow branch.
	big := pgFrame('D', make([]byte, 70000))
	fr := NewPostgreSQLFramer(&chunkRW{chunks: splitBytes(big, 8192)})
	got, err := fr.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame big: %v", err)
	}
	if len(got) != len(big) {
		t.Errorf("frame len = %d, want %d", len(got), len(big))
	}
}

func TestMySQLFramer_ReadFrame_ZeroReadEOF(t *testing.T) {
	// Partial header then a zero-length read -> n==0 EOF branch.
	fr := NewMySQLFramer(&chunkRW{chunks: [][]byte{{0x01}, {}}})
	if _, err := fr.ReadFrame(); !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF from zero read, got %v", err)
	}
}

// --- readFullResponse ---

func newBareLoop(protocol string) *ProxyLoop {
	return &ProxyLoop{protocol: protocol, logger: zap.NewNop()}
}

func TestReadFullResponse_PostgreSQL_ReadyForQuery(t *testing.T) {
	// RowDescription-ish frame + ReadyForQuery terminates the response.
	data := append(pgFrame('T', []byte{0x00}), pgFrame('Z', []byte{'I'})...)
	bc := &BackendConnection{framer: NewPostgreSQLFramer(&chunkRW{chunks: [][]byte{data}})}
	frames, err := newBareLoop("postgresql").readFullResponse(bc)
	if err != nil {
		t.Fatalf("readFullResponse: %v", err)
	}
	if len(frames) != 2 {
		t.Errorf("frame count = %d, want 2", len(frames))
	}
}

func TestReadFullResponse_PostgreSQL_ErrorThenReady(t *testing.T) {
	data := append(pgFrame('E', []byte("fail")), pgFrame('Z', []byte{'I'})...)
	bc := &BackendConnection{framer: NewPostgreSQLFramer(&chunkRW{chunks: [][]byte{data}})}
	frames, err := newBareLoop("postgresql").readFullResponse(bc)
	if err != nil {
		t.Fatalf("readFullResponse: %v", err)
	}
	if len(frames) != 2 {
		t.Errorf("frame count = %d, want 2 (E then Z)", len(frames))
	}
}

func TestReadFullResponse_PostgreSQL_ErrorNoReady(t *testing.T) {
	// ErrorResponse with nothing following -> returns after best-effort extra read.
	bc := &BackendConnection{framer: NewPostgreSQLFramer(&chunkRW{chunks: [][]byte{pgFrame('E', []byte("fail"))}})}
	frames, err := newBareLoop("postgresql").readFullResponse(bc)
	if err != nil {
		t.Fatalf("readFullResponse: %v", err)
	}
	if len(frames) != 1 {
		t.Errorf("frame count = %d, want 1", len(frames))
	}
}

func TestReadFullResponse_PostgreSQL_ReadError(t *testing.T) {
	bc := &BackendConnection{framer: NewPostgreSQLFramer(&chunkRW{readErr: errors.New("boom")})}
	if _, err := newBareLoop("postgresql").readFullResponse(bc); err == nil {
		t.Error("expected read error")
	}
}

func TestReadFullResponse_MySQL_OK(t *testing.T) {
	bc := &BackendConnection{framer: NewMySQLFramer(&chunkRW{chunks: [][]byte{mysqlPacket(1, []byte{0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00})}})}
	frames, err := newBareLoop("mysql").readFullResponse(bc)
	if err != nil {
		t.Fatalf("readFullResponse: %v", err)
	}
	if len(frames) != 1 {
		t.Errorf("frame count = %d, want 1", len(frames))
	}
}

func TestReadFullResponse_MySQL_ReadError(t *testing.T) {
	bc := &BackendConnection{framer: NewMySQLFramer(&chunkRW{readErr: errors.New("boom")})}
	if _, err := newBareLoop("mysql").readFullResponse(bc); err == nil {
		t.Error("expected read error")
	}
}

func TestReadFullResponse_Fallback(t *testing.T) {
	// Unknown protocol -> fallback single-frame read.
	bc := &BackendConnection{framer: NewMySQLFramer(&chunkRW{chunks: [][]byte{mysqlPacket(1, []byte{0xAB})}})}
	frames, err := newBareLoop("other").readFullResponse(bc)
	if err != nil {
		t.Fatalf("readFullResponse fallback: %v", err)
	}
	if len(frames) != 1 {
		t.Errorf("frame count = %d, want 1", len(frames))
	}

	// Fallback read error path.
	bcErr := &BackendConnection{framer: NewMySQLFramer(&chunkRW{readErr: errors.New("boom")})}
	if _, err := newBareLoop("other").readFullResponse(bcErr); err == nil {
		t.Error("expected fallback read error")
	}
}
