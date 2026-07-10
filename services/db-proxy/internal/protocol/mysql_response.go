package protocol

import (
	"encoding/binary"
	"fmt"
)

// MySQLResponseAccumulator handles reading and accumulating MySQL result set responses
// MySQL results can span multiple packets

type MySQLResponseAccumulator struct {
	firstByte  byte
	packets    [][]byte
	isComplete bool
}

// NewMySQLResponseAccumulator creates a new response accumulator
func NewMySQLResponseAccumulator() *MySQLResponseAccumulator {
	return &MySQLResponseAccumulator{
		packets: [][]byte{},
	}
}

// AddPacket adds a packet to the response and determines if complete
// Returns true if the response is complete, false if more packets expected
func (mra *MySQLResponseAccumulator) AddPacket(packet []byte) (bool, error) {
	if len(packet) < 4 {
		return false, fmt.Errorf("packet too short: %d bytes", len(packet))
	}

	// Packet format: 3-byte length (little-endian) + 1-byte sequence + payload
	length := int(packet[0]) | (int(packet[1]) << 8) | (int(packet[2]) << 16)
	// seqNum := packet[3] // sequence number (not used for response logic)
	payload := packet[4:]

	if len(payload) < length {
		return false, fmt.Errorf("incomplete packet: length field says %d, got %d payload bytes", length, len(payload))
	}

	// Store the packet
	mra.packets = append(mra.packets, packet)

	// Determine response type from first packet
	if len(mra.packets) == 1 {
		if len(payload) == 0 {
			return false, fmt.Errorf("empty first packet")
		}
		mra.firstByte = payload[0]
	}

	// Check for completion based on response type
	switch mra.firstByte {
	case 0x00: // OK packet
		// OK packets are single packet (unless it's a slow OK that's part of multi-packet)
		// For now, assume single packet. Multi-packet OK is rare.
		if len(mra.packets) == 1 {
			mra.isComplete = true
			return true, nil
		}

	case 0xFF: // ERR packet
		// Error packets are always single packet
		if len(mra.packets) == 1 {
			mra.isComplete = true
			return true, nil
		}

	case 0xFB: // LOCAL INFILE (deprecated, treat as complete)
		if len(mra.packets) == 1 {
			mra.isComplete = true
			return true, nil
		}

	case 0xFE: // EOF packet or OK packet in new format (MySQL 5.7.5+)
		// Could be:
		// - EOF marker (end of result set or metadata)
		// - ResultSetOK (if deprecate-EOF flag is set)
		// For now, treat first 0xFE as EOF after metadata
		// This is typically the LAST packet
		if len(mra.packets) == 1 {
			// Single EOF (server completed empty result set)
			mra.isComplete = true
			return true, nil
		}
		// Otherwise we need more analysis

	default:
		// 0x01-0xFD: result set header or data row
		// Result set format:
		// 1. Column count (length-encoded integer)
		// 2. Column definitions (one per column, followed by EOF/OK)
		// 3. Rows (one per data row, followed by EOF/OK)
		// 4. EOF/OK packet (signals end)

		// For a simple heuristic: keep reading until we see EOF or OK after data
		return mra.checkResultSetComplete(), nil
	}

	return mra.isComplete, nil
}

// checkResultSetComplete checks if a result set response is complete
// Result sets end with an EOF (0xFE) or OK (0x00) packet
// This is a simplified check - doesn't fully parse the result set structure
func (mra *MySQLResponseAccumulator) checkResultSetComplete() bool {
	if len(mra.packets) < 2 {
		return false
	}

	// Look at the last packet
	lastPacket := mra.packets[len(mra.packets)-1]
	if len(lastPacket) < 5 {
		return false // too short to be a complete packet
	}

	payload := lastPacket[4:]
	if len(payload) == 0 {
		return false
	}

	lastByte := payload[0]

	// Check for EOF (0xFE) or OK (0x00) packet
	// EOF: 0xFE, may have status flags and warning count
	// OK: 0x00
	// ERR: 0xFF (should have been caught earlier)

	if lastByte == 0xFE || lastByte == 0x00 {
		// Could be end of result set
		// For safety, check packet length: if it's short, likely EOF
		length := int(lastPacket[0]) | (int(lastPacket[1]) << 8) | (int(lastPacket[2]) << 16)
		if length <= 10 {
			// Short packet, likely EOF or status packet
			return true
		}
	}

	return false
}

// IsComplete returns whether the response is complete
func (mra *MySQLResponseAccumulator) IsComplete() bool {
	return mra.isComplete
}

// GetPackets returns all accumulated packets
func (mra *MySQLResponseAccumulator) GetPackets() [][]byte {
	return mra.packets
}

// GetAllBytes returns the complete response as a single byte slice
func (mra *MySQLResponseAccumulator) GetAllBytes() []byte {
	var result []byte
	for _, pkt := range mra.packets {
		result = append(result, pkt...)
	}
	return result
}

// ParseResultSetMetadata extracts metadata from a result set
// Returns column count if this is a result set, or 0 if not
func (mra *MySQLResponseAccumulator) ParseResultSetMetadata() (int, error) {
	if len(mra.packets) == 0 {
		return 0, fmt.Errorf("no packets in response")
	}

	firstPacket := mra.packets[0]
	if len(firstPacket) < 5 {
		return 0, fmt.Errorf("first packet too short")
	}

	payload := firstPacket[4:]
	if len(payload) == 0 {
		return 0, fmt.Errorf("empty payload")
	}

	// Check first byte to determine response type
	switch payload[0] {
	case 0x00:
		// OK packet - not a result set
		return 0, nil
	case 0xFF:
		// Error packet - not a result set
		return 0, fmt.Errorf("error response")
	case 0xFE:
		// EOF packet - not a result set
		return 0, nil
	default:
		// Result set header - first byte is column count (length-encoded)
		colCount, n := decodeLengthEncodedInt(payload)
		if n == 0 {
			return 0, fmt.Errorf("failed to decode column count")
		}
		return colCount, nil
	}
}

// decodeLengthEncodedInt decodes a MySQL length-encoded integer
// Returns (value, bytesRead)
func decodeLengthEncodedInt(data []byte) (int, int) {
	if len(data) == 0 {
		return 0, 0
	}

	firstByte := data[0]

	switch {
	case firstByte < 0xFB:
		// Value is first byte itself
		return int(firstByte), 1
	case firstByte == 0xFB:
		// NULL value
		return -1, 1
	case firstByte == 0xFC:
		// 2 bytes follow
		if len(data) < 3 {
			return 0, 0
		}
		return int(binary.LittleEndian.Uint16(data[1:3])), 3
	case firstByte == 0xFD:
		// 3 bytes follow
		if len(data) < 4 {
			return 0, 0
		}
		// Little-endian, need to manually read 3 bytes
		return int(data[1]) | (int(data[2]) << 8) | (int(data[3]) << 16), 4
	case firstByte == 0xFE:
		// 8 bytes follow
		if len(data) < 9 {
			return 0, 0
		}
		return int(binary.LittleEndian.Uint64(data[1:9])), 9
	default:
		return 0, 0
	}
}
