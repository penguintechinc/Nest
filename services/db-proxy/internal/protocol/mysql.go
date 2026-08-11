package protocol

import "fmt"

const (
	// MySQL command type constants
	MySQLComQuery = 0x03 // COM_QUERY
)

// MySQLParser parses MySQL protocol messages
type MySQLParser struct{}

func (m *MySQLParser) Protocol() string {
	return "mysql"
}

// Parse extracts the SQL query from a MySQL protocol message
// MySQL packet format:
//   - 3 bytes: packet length (little-endian)
//   - 1 byte: sequence number
//   - N bytes: payload
//   - COM_QUERY (0x03) + query text
func (m *MySQLParser) Parse(data []byte) (*ParsedQuery, error) {
	if len(data) < 5 {
		// Too short to be a valid MySQL message
		return &ParsedQuery{
			Protocol:  "mysql",
			QueryType: QueryTypeUnknown,
			RawBytes:  data,
		}, nil
	}

	// Skip header if present (packet length + sequence number)
	// For simplicity, check if we have a command byte
	var payloadStart int
	var commandByte byte

	// Try to parse as a complete packet with header
	if len(data) >= 4 {
		// Packet format: length(3) + seqnum(1) + payload
		// We don't validate length here, just try to find the command
		commandByte = data[4]
		payloadStart = 5
	} else if len(data) >= 1 {
		// Assume raw payload without header
		commandByte = data[0]
		payloadStart = 1
	} else {
		return &ParsedQuery{
			Protocol:  "mysql",
			QueryType: QueryTypeUnknown,
			RawBytes:  data,
		}, nil
	}

	// Handle COM_QUERY
	if commandByte == MySQLComQuery {
		if payloadStart >= len(data) {
			// No query text after command byte
			return &ParsedQuery{
				Protocol:  "mysql",
				QueryType: QueryTypeUnknown,
				RawBytes:  data,
			}, nil
		}

		queryText := string(data[payloadStart:])
		queryType := ClassifyQuery(queryText)

		return &ParsedQuery{
			Protocol:  "mysql",
			QueryText: queryText,
			QueryType: queryType,
			RawBytes:  data,
		}, nil
	}

	// Other command types: treat as WRITE (fail-safe)
	return &ParsedQuery{
		Protocol:  "mysql",
		QueryText: "",
		QueryType: QueryTypeUnknown,
		RawBytes:  data,
	}, fmt.Errorf("unsupported MySQL command: 0x%02x", commandByte)
}
