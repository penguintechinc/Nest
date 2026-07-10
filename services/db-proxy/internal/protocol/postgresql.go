package protocol

import (
	"fmt"
	"strings"
)

const (
	// PostgreSQL message types
	PgMsgQuery = 'Q' // Simple query
	PgMsgParse = 'P' // Parse (extended protocol - deferred)
)

// PostgreSQLParser parses PostgreSQL protocol messages
type PostgreSQLParser struct{}

func (p *PostgreSQLParser) Protocol() string {
	return "postgresql"
}

// Parse extracts the SQL query from a PostgreSQL protocol message
// PostgreSQL message format (extended):
//   - 1 byte: message type ('Q' for simple query, 'P' for parse, etc.)
//   - 4 bytes: message length (big-endian, including the length itself)
//   - N bytes: message payload
//
// Simple Query ('Q'):
//   - Message type: 'Q'
//   - Length: int32
//   - Query string: null-terminated
//
// DEFERRED: Full support for extended protocol (Parse + Bind + Execute).
// For now, we recognize 'P' messages and route them to PRIMARY (fail-safe).
func (p *PostgreSQLParser) Parse(data []byte) (*ParsedQuery, error) {
	if len(data) < 5 {
		// Too short for a valid PostgreSQL message header
		return &ParsedQuery{
			Protocol:  "postgresql",
			QueryType: QueryTypeUnknown,
			RawBytes:  data,
		}, nil
	}

	msgType := data[0]

	// Handle simple query message ('Q')
	if msgType == PgMsgQuery {
		// Format: Q + length(4) + query_string(null-terminated)
		// Length is big-endian and includes the 4 bytes of length itself
		if len(data) < 5 {
			return &ParsedQuery{
				Protocol:  "postgresql",
				QueryType: QueryTypeUnknown,
				RawBytes:  data,
			}, nil
		}

		// Read message length (big-endian)
		msgLen := int((uint32(data[1]) << 24) | (uint32(data[2]) << 16) | (uint32(data[3]) << 8) | uint32(data[4]))

		// Extract query string (everything after the length field, minus the null terminator)
		payloadStart := 5
		payloadEnd := payloadStart + msgLen - 4 // msgLen includes the 4-byte length field

		if payloadEnd > len(data) {
			// Incomplete message, treat as-is
			payloadEnd = len(data)
		}

		if payloadStart >= len(data) {
			return &ParsedQuery{
				Protocol:  "postgresql",
				QueryType: QueryTypeUnknown,
				RawBytes:  data,
			}, nil
		}

		queryPayload := data[payloadStart:payloadEnd]

		// Remove null terminator if present
		queryText := strings.TrimRight(string(queryPayload), "\x00")
		queryType := ClassifyQuery(queryText)

		return &ParsedQuery{
			Protocol:  "postgresql",
			QueryText: queryText,
			QueryType: queryType,
			RawBytes:  data,
		}, nil
	}

	// Handle extended protocol messages (Parse = 'P')
	// DEFERRED: Full prepared statement support.
	// For now: route to PRIMARY (fail-safe), mark as deferred.
	if msgType == PgMsgParse {
		return &ParsedQuery{
			Protocol:   "postgresql",
			QueryText:  "",
			QueryType:  QueryTypeUnknown, // Treat as UNKNOWN → routes to PRIMARY
			IsExtended: true,
			RawBytes:   data,
		}, fmt.Errorf("PostgreSQL extended protocol (Parse/Bind/Execute) not yet supported; routing to primary")
	}

	// Unknown message type: treat as WRITE
	return &ParsedQuery{
		Protocol:  "postgresql",
		QueryText: "",
		QueryType: QueryTypeUnknown,
		RawBytes:  data,
	}, fmt.Errorf("unknown PostgreSQL message type: %c (0x%02x)", msgType, msgType)
}
