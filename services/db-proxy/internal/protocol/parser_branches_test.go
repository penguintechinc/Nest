package protocol

import "testing"

func TestMySQLParser_Parse_Branches(t *testing.T) {
	p := NewParser("mysql")

	// Too short (<5 bytes) -> Unknown, no error.
	if pq, err := p.Parse([]byte{0x01, 0x02}); err != nil || pq.QueryType != QueryTypeUnknown {
		t.Errorf("short packet = (%v, %v)", pq.QueryType, err)
	}

	// COM_QUERY with text.
	q := mysqlPacket(0, append([]byte{MySQLComQuery}, []byte("SELECT 1")...))
	if pq, err := p.Parse(q); err != nil || pq.QueryText != "SELECT 1" {
		t.Errorf("COM_QUERY = (%q, %v)", pq.QueryText, err)
	}

	// COM_QUERY with no trailing text (payloadStart >= len) -> Unknown.
	qEmpty := mysqlPacket(0, []byte{MySQLComQuery})
	if pq, err := p.Parse(qEmpty); err != nil || pq.QueryType != QueryTypeUnknown {
		t.Errorf("empty COM_QUERY = (%v, %v)", pq.QueryType, err)
	}

	// Non-query command -> error, fail-safe Unknown.
	other := mysqlPacket(0, []byte{0x0E, 0x00}) // COM_PING-ish
	if pq, err := p.Parse(other); err == nil || pq.QueryType != QueryTypeUnknown {
		t.Errorf("other command = (%v, %v), want error", pq.QueryType, err)
	}
}

func pgQueryFrame(text string) []byte {
	payload := append([]byte(text), 0x00) // null-terminated
	msgLen := 4 + len(payload)
	return append([]byte{
		PgMsgQuery,
		byte(msgLen >> 24), byte(msgLen >> 16), byte(msgLen >> 8), byte(msgLen),
	}, payload...)
}

func TestPostgreSQLParser_Parse_Branches(t *testing.T) {
	p := NewParser("postgresql")

	// Too short.
	if pq, err := p.Parse([]byte{'Q', 0x00}); err != nil || pq.QueryType != QueryTypeUnknown {
		t.Errorf("short = (%v, %v)", pq.QueryType, err)
	}

	// Simple query.
	if pq, err := p.Parse(pgQueryFrame("SELECT 1")); err != nil || pq.QueryText != "SELECT 1" {
		t.Errorf("simple query = (%q, %v)", pq.QueryText, err)
	}

	// payloadEnd overrun -> clamped to len(data), still parses.
	over := []byte{PgMsgQuery, 0x00, 0x00, 0x00, 0x64, 'S', 'E', 'L', 'E', 'C', 'T', 0x00} // msgLen=100 but data short
	if pq, err := p.Parse(over); err != nil || pq.QueryType == QueryTypeUnknown {
		t.Errorf("overrun query = (%v, %v)", pq.QueryType, err)
	}

	// Extended-protocol 'P' -> error, IsExtended.
	pmsg := []byte{PgMsgParse, 0x00, 0x00, 0x00, 0x04, 0x00}
	if pq, err := p.Parse(pmsg); err == nil || !pq.IsExtended {
		t.Errorf("parse msg = (%v, %v), want error+extended", pq.IsExtended, err)
	}

	// Unknown message type -> error.
	if pq, err := p.Parse([]byte{'Z', 0x00, 0x00, 0x00, 0x04}); err == nil || pq.QueryType != QueryTypeUnknown {
		t.Errorf("unknown msg = (%v, %v), want error", pq.QueryType, err)
	}
}
