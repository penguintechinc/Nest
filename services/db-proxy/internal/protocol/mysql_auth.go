package protocol

import (
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
)

// MySQL authentication handler for mysql_native_password
// Based on MySQL protocol documentation

const (
	// MySQL protocol constants
	MySQLProtoVersion = 10

	// Capability flags
	CapabilityClientLongPassword              = 0x0001
	CapabilityClientFoundRows                 = 0x0002
	CapabilityClientLongFlag                  = 0x0004
	CapabilityClientConnectWithDB             = 0x0008
	CapabilityClientCompress                  = 0x0020
	CapabilityClientODBC                      = 0x0040
	CapabilityClientLocalFiles                = 0x0080
	CapabilityClientIgnoreSpace               = 0x0100
	CapabilityClientChangeUser                = 0x0200
	CapabilityClientInteractive               = 0x0400
	CapabilityClientSSL                       = 0x0800
	CapabilityClientIgnoreSigpipe             = 0x1000
	CapabilityClientTransactions              = 0x2000
	CapabilityClientReserved                  = 0x4000
	CapabilityClientSecureConn                = 0x8000
	CapabilityClientMultiStatements           = 0x00010000
	CapabilityClientMultiResults              = 0x00020000
	CapabilityClientPluginAuth                = 0x00080000
	CapabilityClientConnAttrs                 = 0x00100000
	CapabilityClientPluginAuthLenenc          = 0x00200000
	CapabilityClientCanHandleExpiredPasswords = 0x00400000
	CapabilityClientSessionTrack              = 0x00800000
	CapabilityClientDeprecateEOF              = 0x01000000

	// Default capability flags for proxy
	DefaultCapabilities = CapabilityClientLongPassword |
		CapabilityClientFoundRows |
		CapabilityClientLongFlag |
		CapabilityClientConnectWithDB |
		CapabilityClientODBC |
		CapabilityClientTransactions |
		CapabilityClientSecureConn |
		CapabilityClientPluginAuth |
		CapabilityClientPluginAuthLenenc

	// Auth plugin names
	AuthPluginMysqlNativePassword = "mysql_native_password"
	AuthPluginCachingSha2Password = "caching_sha2_password"
)

// MySQLHandshake represents the initial server handshake packet
type MySQLHandshake struct {
	ProtoVersion         uint8
	ServerVersion        string
	ConnectionID         uint32
	AuthPluginData1      []byte // First 8 bytes of salt
	Filler1              byte
	CapabilityFlags      uint32 // Lower 2 bytes
	CharacterSet         uint8
	StatusFlags          uint16
	CapabilityFlagsUpper uint16 // Upper 2 bytes
	AuthPluginDataLen    uint8
	Reserved             [10]byte
	AuthPluginData2      []byte // Rest of salt (minimum 13 bytes total with data1)
	AuthPluginName       string
}

// MySQLAuthHandler handles MySQL authentication
type MySQLAuthHandler struct {
	backendPassword string
	handshake       *MySQLHandshake
}

// NewMySQLAuthHandler creates a new MySQL auth handler
func NewMySQLAuthHandler(password string) *MySQLAuthHandler {
	return &MySQLAuthHandler{
		backendPassword: password,
	}
}

// HandleHandshake reads the server's Handshake packet and returns it
func (ah *MySQLAuthHandler) HandleHandshake(conn io.Reader) (*MySQLHandshake, error) {
	// Read packet length (3 bytes, little-endian)
	lenBuf := make([]byte, 3)
	_, err := io.ReadFull(conn, lenBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read handshake length: %w", err)
	}
	length := int(lenBuf[0]) | (int(lenBuf[1]) << 8) | (int(lenBuf[2]) << 16)

	// Read sequence number (1 byte)
	seqBuf := make([]byte, 1)
	_, err = io.ReadFull(conn, seqBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to read handshake sequence: %w", err)
	}

	// Read payload
	payload := make([]byte, length)
	_, err = io.ReadFull(conn, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to read handshake payload: %w", err)
	}

	// Parse handshake
	hs, err := ah.parseHandshake(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse handshake: %w", err)
	}

	ah.handshake = hs
	return hs, nil
}

// parseHandshake parses a MySQL Handshake packet payload
func (ah *MySQLAuthHandler) parseHandshake(payload []byte) (*MySQLHandshake, error) {
	if len(payload) < 10 {
		return nil, fmt.Errorf("handshake payload too short: %d", len(payload))
	}

	pos := 0
	hs := &MySQLHandshake{}

	// Protocol version
	hs.ProtoVersion = payload[pos]
	pos++

	// Server version (null-terminated string)
	nullPos := pos
	for nullPos < len(payload) && payload[nullPos] != 0 {
		nullPos++
	}
	if nullPos >= len(payload) {
		return nil, fmt.Errorf("malformed server version in handshake")
	}
	hs.ServerVersion = string(payload[pos:nullPos])
	pos = nullPos + 1

	// Connection ID (4 bytes, little-endian)
	if pos+4 > len(payload) {
		return nil, fmt.Errorf("handshake too short for connection ID")
	}
	hs.ConnectionID = binary.LittleEndian.Uint32(payload[pos : pos+4])
	pos += 4

	// Auth plugin data part 1 (8 bytes)
	if pos+8 > len(payload) {
		return nil, fmt.Errorf("handshake too short for auth plugin data 1")
	}
	hs.AuthPluginData1 = payload[pos : pos+8]
	pos += 8

	// Filler (1 byte, always 0)
	if pos+1 > len(payload) {
		return nil, fmt.Errorf("handshake too short for filler")
	}
	hs.Filler1 = payload[pos]
	pos++

	// Capability flags lower 2 bytes (2 bytes, little-endian)
	if pos+2 > len(payload) {
		return nil, fmt.Errorf("handshake too short for capability flags lower")
	}
	hs.CapabilityFlags = uint32(binary.LittleEndian.Uint16(payload[pos : pos+2]))
	pos += 2

	// Character set (1 byte)
	if pos+1 > len(payload) {
		return nil, fmt.Errorf("handshake too short for character set")
	}
	hs.CharacterSet = payload[pos]
	pos++

	// Status flags (2 bytes, little-endian)
	if pos+2 > len(payload) {
		return nil, fmt.Errorf("handshake too short for status flags")
	}
	hs.StatusFlags = binary.LittleEndian.Uint16(payload[pos : pos+2])
	pos += 2

	// Capability flags upper 2 bytes (2 bytes, little-endian)
	if pos+2 > len(payload) {
		return nil, fmt.Errorf("handshake too short for capability flags upper")
	}
	hs.CapabilityFlagsUpper = binary.LittleEndian.Uint16(payload[pos : pos+2])
	pos += 2

	// Auth plugin data length (1 byte)
	if pos+1 > len(payload) {
		return nil, fmt.Errorf("handshake too short for auth plugin data length")
	}
	hs.AuthPluginDataLen = payload[pos]
	pos++

	// Reserved (10 bytes)
	if pos+10 > len(payload) {
		return nil, fmt.Errorf("handshake too short for reserved")
	}
	copy(hs.Reserved[:], payload[pos:pos+10])
	pos += 10

	// Auth plugin data part 2 (at least 13 bytes total with part 1)
	// Part 2 is (authPluginDataLen - 8) bytes, with a null terminator
	if hs.AuthPluginDataLen > 0 {
		part2Len := int(hs.AuthPluginDataLen) - 8
		if part2Len < 0 {
			part2Len = 0
		}
		if pos+part2Len > len(payload) {
			return nil, fmt.Errorf("handshake too short for auth plugin data 2: need %d, have %d", part2Len, len(payload)-pos)
		}
		// Remove trailing null terminator from part 2
		part2Data := payload[pos : pos+part2Len]
		if len(part2Data) > 0 && part2Data[len(part2Data)-1] == 0 {
			hs.AuthPluginData2 = part2Data[:len(part2Data)-1]
		} else {
			hs.AuthPluginData2 = part2Data
		}
		pos += part2Len
	}

	// Auth plugin name (null-terminated string)
	if pos < len(payload) {
		nullPos := pos
		for nullPos < len(payload) && payload[nullPos] != 0 {
			nullPos++
		}
		hs.AuthPluginName = string(payload[pos:nullPos])
	}

	return hs, nil
}

// SendHandshakeResponse sends a HandshakeResponse packet for mysql_native_password auth
func (ah *MySQLAuthHandler) SendHandshakeResponse(conn io.Writer, user, database string) error {
	if ah.handshake == nil {
		return fmt.Errorf("handshake not received")
	}

	// Build the auth response using mysql_native_password
	authResponse := ah.calculateNativePasswordAuth(ah.backendPassword, ah.handshake.AuthPluginData1, ah.handshake.AuthPluginData2)

	// Build HandshakeResponse packet payload
	payload := []byte{}

	// Capability flags (4 bytes, little-endian) - client side
	clientCaps := DefaultCapabilities
	payload = append(payload, byte(clientCaps), byte(clientCaps>>8), byte(clientCaps>>16), byte(clientCaps>>24))

	// Max packet size (4 bytes, little-endian) - default 16MB
	maxPacket := uint32(16 * 1024 * 1024)
	payload = append(payload, byte(maxPacket), byte(maxPacket>>8), byte(maxPacket>>16), byte(maxPacket>>24))

	// Character set (1 byte) - use utf8mb4 (255)
	payload = append(payload, 255)

	// Filler (23 bytes of zeros)
	payload = append(payload, make([]byte, 23)...)

	// Username (null-terminated)
	payload = append(payload, []byte(user)...)
	payload = append(payload, 0)

	// Auth response (length-encoded)
	// For mysql_native_password, response is 20 bytes
	payload = append(payload, byte(len(authResponse)))
	payload = append(payload, authResponse...)

	// Database name (null-terminated)
	if database != "" {
		payload = append(payload, []byte(database)...)
		payload = append(payload, 0)
	}

	// Client auth plugin name (null-terminated)
	payload = append(payload, []byte(AuthPluginMysqlNativePassword)...)
	payload = append(payload, 0)

	// Build complete packet with header
	packet := ah.buildPacket(payload, 1) // sequence number 1

	_, err := conn.Write(packet)
	return err
}

// buildPacket builds a MySQL packet with the given payload and sequence number
func (ah *MySQLAuthHandler) buildPacket(payload []byte, seqNum byte) []byte {
	// 3-byte length (little-endian)
	length := len(payload)
	packet := []byte{
		byte(length),
		byte(length >> 8),
		byte(length >> 16),
		seqNum,
	}
	packet = append(packet, payload...)
	return packet
}

// ReadAuthResult reads the server's response to the handshake
// Returns nil error if auth OK, error otherwise
func (ah *MySQLAuthHandler) ReadAuthResult(conn io.Reader) error {
	// Read packet length
	lenBuf := make([]byte, 3)
	_, err := io.ReadFull(conn, lenBuf)
	if err != nil {
		return fmt.Errorf("failed to read auth result length: %w", err)
	}
	length := int(lenBuf[0]) | (int(lenBuf[1]) << 8) | (int(lenBuf[2]) << 16)

	// Read sequence number
	seqBuf := make([]byte, 1)
	_, err = io.ReadFull(conn, seqBuf)
	if err != nil {
		return fmt.Errorf("failed to read auth result sequence: %w", err)
	}

	// Read payload
	payload := make([]byte, length)
	_, err = io.ReadFull(conn, payload)
	if err != nil {
		return fmt.Errorf("failed to read auth result payload: %w", err)
	}

	// Parse response
	if len(payload) == 0 {
		return fmt.Errorf("empty auth result payload")
	}

	// First byte indicates packet type
	packetType := payload[0]

	switch packetType {
	case 0x00: // OK packet
		return nil
	case 0xFF: // ERR packet
		// Parse error message
		if len(payload) > 1 {
			// Bytes 1-2: error code (little-endian)
			// Byte 3: SQL state marker (should be '#')
			// Bytes 4-8: SQL state
			// Remaining: error message
			errorMsg := string(payload[1:])
			return fmt.Errorf("auth error: %s", errorMsg)
		}
		return fmt.Errorf("auth error: unknown")
	default:
		return fmt.Errorf("unexpected auth result packet type: 0x%02x", packetType)
	}
}

// calculateNativePasswordAuth calculates the mysql_native_password hash
// Formula: SHA1(SHA1(password) + salt) XOR SHA1(password)
func (ah *MySQLAuthHandler) calculateNativePasswordAuth(password string, salt1, salt2 []byte) []byte {
	// Combine salt (total 20 bytes for mysql_native_password)
	salt := append(salt1, salt2...)

	// Step 1: SHA1(password)
	sha1pw := sha1.Sum([]byte(password))

	// Step 2: SHA1(SHA1(password) + salt)
	h := sha1.New()
	h.Write(sha1pw[:])
	h.Write(salt)
	sha1PwSalt := h.Sum(nil)

	// Step 3: XOR SHA1(SHA1(password) + salt) with SHA1(password)
	result := make([]byte, 20)
	for i := 0; i < 20; i++ {
		result[i] = sha1PwSalt[i] ^ sha1pw[i]
	}

	return result
}
