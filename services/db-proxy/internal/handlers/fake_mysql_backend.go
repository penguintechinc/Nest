package handlers

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	protocolpkg "github.com/penguintechinc/nest/services/db-proxy/internal/protocol"
)

// FakeMySQLBackend simulates a minimal MySQL server for testing
// Implements just enough of the protocol to handle Handshake → HandshakeResponse → auth result
type FakeMySQLBackend struct {
	listener     net.Listener
	Port         int
	Name         string
	ReceivedData [][]byte // all queries received
	mu           sync.Mutex
	done         chan struct{}
	password     string // backend password for testing
}

// NewFakeMySQLBackend starts a fake MySQL server on a random port
func NewFakeMySQLBackend(name string, password string) (*FakeMySQLBackend, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	addr := listener.Addr().(*net.TCPAddr)

	server := &FakeMySQLBackend{
		listener: listener,
		Port:     addr.Port,
		Name:     name,
		password: password,
		done:     make(chan struct{}),
	}

	// Start accepting connections
	go func() {
		for {
			select {
			case <-server.done:
				return
			default:
			}

			conn, err := listener.Accept()
			if err != nil {
				return
			}

			go server.handleConnection(conn)
		}
	}()

	return server, nil
}

// handleConnection handles a single client connection
func (fmb *FakeMySQLBackend) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set read/write deadline to avoid hanging tests
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send Handshake packet
	handshakePacket := fmb.buildHandshakePacket()
	_, err := conn.Write(handshakePacket)
	if err != nil {
		return
	}

	// Read HandshakeResponse packet
	hs, err := fmb.readHandshakeResponse(conn)
	if err != nil {
		// Send error response
		errPacket := fmb.buildErrorPacket(2, "Access denied for user")
		conn.Write(errPacket)
		return
	}

	// Verify password if configured
	if fmb.password != "" && !fmb.verifyPassword(hs.authResponse) {
		errPacket := fmb.buildErrorPacket(2, "Access denied for user")
		conn.Write(errPacket)
		return
	}

	// Send OK packet
	okPacket := fmb.buildOKPacket(2, 0, 0, 0)
	_, err = conn.Write(okPacket)
	if err != nil {
		return
	}

	// Now handle queries
	fmb.handleQueryLoop(conn)
}

// handleQueryLoop processes query packets from the client
func (fmb *FakeMySQLBackend) handleQueryLoop(conn net.Conn) {
	for {
		// Read query packet
		packet, err := fmb.readPacket(conn)
		if err != nil {
			return
		}

		if len(packet) < 4 {
			continue
		}

		// Parse packet header
		seqNum := packet[3]
		payload := packet[4:]

		if len(payload) == 0 {
			continue
		}

		commandType := payload[0]

		switch commandType {
		case 0x03: // COM_QUERY
			// Extract query string
			queryStr := string(payload[1:])
			if queryStr != "" {
				fmb.mu.Lock()
				fmb.ReceivedData = append(fmb.ReceivedData, []byte(queryStr))
				fmb.mu.Unlock()

				// Send response: OK packet with command completion
				okPacket := fmb.buildOKPacket(seqNum+1, 0, 0, 0)
				conn.Write(okPacket)
			}

		case 0x01: // COM_QUIT
			return

		default:
			// For other commands, send error
			errPacket := fmb.buildErrorPacket(seqNum+1, "Command not supported")
			conn.Write(errPacket)
		}
	}
}

// readPacket reads one complete MySQL packet from the connection
func (fmb *FakeMySQLBackend) readPacket(conn net.Conn) ([]byte, error) {
	// Read packet header (3-byte length + 1-byte sequence)
	header := make([]byte, 4)
	_, err := io.ReadFull(conn, header)
	if err != nil {
		return nil, err
	}

	length := int(header[0]) | (int(header[1]) << 8) | (int(header[2]) << 16)
	seqNum := header[3]

	// Read payload
	payload := make([]byte, length)
	_, err = io.ReadFull(conn, payload)
	if err != nil {
		return nil, err
	}

	// Reconstruct packet
	packet := []byte{header[0], header[1], header[2], seqNum}
	packet = append(packet, payload...)

	return packet, nil
}

// readHandshakeResponse reads the client's HandshakeResponse packet
type handshakeResponse struct {
	user         string
	authResponse []byte
	database     string
}

func (fmb *FakeMySQLBackend) readHandshakeResponse(conn net.Conn) (*handshakeResponse, error) {
	packet, err := fmb.readPacket(conn)
	if err != nil {
		return nil, err
	}

	if len(packet) < 4 {
		return nil, fmt.Errorf("handshake response too short")
	}

	payload := packet[4:]
	if len(payload) < 32 {
		return nil, fmt.Errorf("handshake response payload too short")
	}

	pos := 0

	// Skip capability flags (4 bytes)
	pos += 4

	// Skip max packet size (4 bytes)
	pos += 4

	// Skip character set (1 byte)
	pos++

	// Skip filler (23 bytes)
	pos += 23

	// Read username (null-terminated)
	userEnd := pos
	for userEnd < len(payload) && payload[userEnd] != 0 {
		userEnd++
	}
	user := string(payload[pos:userEnd])
	pos = userEnd + 1

	// Read auth response (length-encoded)
	var authResponse []byte
	if pos < len(payload) {
		authLen := int(payload[pos])
		pos++
		if pos+authLen <= len(payload) {
			authResponse = payload[pos : pos+authLen]
			pos += authLen
		}
	}

	// Read database (null-terminated)
	var database string
	if pos < len(payload) {
		dbEnd := pos
		for dbEnd < len(payload) && payload[dbEnd] != 0 {
			dbEnd++
		}
		database = string(payload[pos:dbEnd])
	}

	return &handshakeResponse{
		user:         user,
		authResponse: authResponse,
		database:     database,
	}, nil
}

// verifyPassword verifies the client's auth response using mysql_native_password
func (fmb *FakeMySQLBackend) verifyPassword(clientAuth []byte) bool {
	if fmb.password == "" {
		return true // no password check if not configured
	}

	// For testing, we just accept any auth
	// In a real implementation, we'd verify the SHA1 hash
	// clientAuth should be: SHA1(SHA1(password) + salt) XOR SHA1(password)
	// We'd need to verify it matches what we expect
	return len(clientAuth) > 0 // just check that we got something
}

// buildHandshakePacket builds the initial Handshake packet
func (fmb *FakeMySQLBackend) buildHandshakePacket() []byte {
	payload := []byte{}

	// Protocol version
	payload = append(payload, protocolpkg.MySQLProtoVersion)

	// Server version
	serverVersion := "8.0.0-test"
	payload = append(payload, []byte(serverVersion)...)
	payload = append(payload, 0)

	// Connection ID (4 bytes, little-endian)
	payload = append(payload, 1, 0, 0, 0)

	// Auth plugin data part 1 (8 bytes) - salt
	salt1 := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	payload = append(payload, salt1...)

	// Filler (1 byte, always 0)
	payload = append(payload, 0)

	// Capability flags lower 2 bytes
	caps := protocolpkg.DefaultCapabilities
	payload = append(payload, byte(caps), byte(caps>>8))

	// Character set
	payload = append(payload, 255) // utf8mb4

	// Status flags
	payload = append(payload, 0x02, 0x00) // AUTOCOMMIT

	// Capability flags upper 2 bytes
	payload = append(payload, byte(caps>>16), byte(caps>>24))

	// Auth plugin data length
	payload = append(payload, 21) // min 13 bytes total salt

	// Reserved (10 bytes)
	payload = append(payload, make([]byte, 10)...)

	// Auth plugin data part 2 (additional salt bytes)
	salt2 := []byte{0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13, 0x00}
	payload = append(payload, salt2...)

	// Auth plugin name
	payload = append(payload, []byte(protocolpkg.AuthPluginMysqlNativePassword)...)
	payload = append(payload, 0)

	return fmb.buildPacket(payload, 0)
}

// buildOKPacket builds an OK packet
func (fmb *FakeMySQLBackend) buildOKPacket(seqNum byte, affectedRows, lastInsertID, statusFlags uint16) []byte {
	payload := []byte{}

	// Header
	payload = append(payload, 0x00) // OK packet indicator

	// Affected rows (1 byte for now)
	payload = append(payload, byte(affectedRows))

	// Last insert ID (1 byte for now)
	payload = append(payload, byte(lastInsertID))

	// Status flags (2 bytes, little-endian)
	payload = append(payload, byte(statusFlags), byte(statusFlags>>8))

	// Warning count (2 bytes)
	payload = append(payload, 0, 0)

	// Optional: server message (for now, empty)

	return fmb.buildPacket(payload, seqNum)
}

// buildErrorPacket builds an ERR packet
func (fmb *FakeMySQLBackend) buildErrorPacket(seqNum byte, message string) []byte {
	payload := []byte{}

	// Header
	payload = append(payload, 0xFF) // ERR packet indicator

	// Error code (2 bytes, little-endian) - use 1045 (access denied)
	errorCode := uint16(1045)
	payload = append(payload, byte(errorCode&0xFF), byte((errorCode>>8)&0xFF))

	// SQL state marker
	payload = append(payload, '#')

	// SQL state (5 bytes)
	payload = append(payload, []byte("28000")...)

	// Error message
	payload = append(payload, []byte(message)...)

	return fmb.buildPacket(payload, seqNum)
}

// buildPacket builds a complete MySQL packet
func (fmb *FakeMySQLBackend) buildPacket(payload []byte, seqNum byte) []byte {
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

// Close stops the fake backend server
func (fmb *FakeMySQLBackend) Close() {
	close(fmb.done)
	fmb.listener.Close()
}

// GetReceivedQueries returns all queries received
func (fmb *FakeMySQLBackend) GetReceivedQueries() []string {
	fmb.mu.Lock()
	defer fmb.mu.Unlock()

	queries := make([]string, len(fmb.ReceivedData))
	for i, data := range fmb.ReceivedData {
		queries[i] = strings.TrimSpace(string(data))
	}
	return queries
}
