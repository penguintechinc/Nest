package handlers

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

// FakePostgreSQLBackend simulates a minimal PostgreSQL server for testing
// Implements just enough of the protocol to handle StartupMessage → auth → ReadyForQuery
type FakePostgreSQLBackend struct {
	listener     net.Listener
	Port         int
	Name         string
	ReceivedData [][]byte // all queries received
	mu           sync.Mutex
	done         chan struct{}
}

// NewFakePostgreSQLBackend starts a fake PostgreSQL server on a random port
func NewFakePostgreSQLBackend(name string) (*FakePostgreSQLBackend, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	addr := listener.Addr().(*net.TCPAddr)

	server := &FakePostgreSQLBackend{
		listener: listener,
		Port:     addr.Port,
		Name:     name,
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
func (fpb *FakePostgreSQLBackend) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Read and handle StartupMessage
	// StartupMessage format: length(4) + protocol_version(4) + params...
	// Unlike other messages, it does NOT have a message type byte
	startupMsg, err := fpb.readStartupMessage(conn)
	if err != nil {
		return
	}

	// Verify it's a valid StartupMessage (begins with protocol version)
	if len(startupMsg) < 4 {
		return
	}

	// For testing, we accept auth without checking user
	// Send AuthenticationOk
	authOkMsg := []byte{0x52}                             // 'R' for Authentication message
	authOkMsg = append(authOkMsg, 0x00, 0x00, 0x00, 0x08) // length: 4 (length field) + 4 (auth type)
	authOkMsg = append(authOkMsg, 0x00, 0x00, 0x00, 0x00) // AuthenticationOk = 0
	conn.Write(authOkMsg)

	// Send ParameterStatus (for environment setup)
	paramMsg := fpb.buildParameterStatus("application_name", "db-proxy-test")
	conn.Write(paramMsg)

	// Send BackendKeyData (for cancel/notice key)
	backendKeyMsg := fpb.buildBackendKeyData(123, 456)
	conn.Write(backendKeyMsg)

	// Send ReadyForQuery
	readyMsg := fpb.buildReadyForQuery('I') // 'I' = idle
	conn.Write(readyMsg)

	// Now handle queries
	fpb.handleQueryLoop(conn)
}

// handleQueryLoop processes Query messages from the client
func (fpb *FakePostgreSQLBackend) handleQueryLoop(conn net.Conn) {
	for {
		msg, err := fpb.readMessage(conn)
		if err != nil {
			return
		}

		if len(msg) < 1 {
			continue
		}

		msgType := msg[0]

		switch msgType {
		case 'Q': // Query message
			// Extract query string (skip the 'Q' and length field, find null terminator)
			queryStr := fpb.extractQueryString(msg)
			if queryStr != "" {
				fpb.mu.Lock()
				fpb.ReceivedData = append(fpb.ReceivedData, []byte(queryStr))
				fpb.mu.Unlock()

				// Send a simple response: RowDescription + CommandComplete + ReadyForQuery
				// For test purposes, just send CommandComplete
				cmdCompleteMsg := fpb.buildCommandComplete("SELECT 1")
				conn.Write(cmdCompleteMsg)

				readyMsg := fpb.buildReadyForQuery('I')
				conn.Write(readyMsg)
			}

		case 'X': // Terminate
			return

		default:
			// For other message types, send ErrorResponse and continue
			errMsg := fpb.buildErrorResponse("Unsupported message type")
			conn.Write(errMsg)

			readyMsg := fpb.buildReadyForQuery('I')
			conn.Write(readyMsg)
		}
	}
}

// readStartupMessage reads a PostgreSQL StartupMessage
// StartupMessage format (special - no type byte): length(4) + protocol_version(4) + params...
func (fpb *FakePostgreSQLBackend) readStartupMessage(conn net.Conn) ([]byte, error) {
	// Read message length (4 bytes, big-endian)
	lenBuf := make([]byte, 4)
	_, err := io.ReadFull(conn, lenBuf)
	if err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)

	// Read payload (msgLen - 4, since length includes itself)
	if msgLen < 4 {
		return nil, fmt.Errorf("invalid message length: %d", msgLen)
	}

	payload := make([]byte, msgLen-4)
	_, err = io.ReadFull(conn, payload)
	if err != nil {
		return nil, err
	}

	// Reconstruct the full message (without type byte)
	result := lenBuf
	result = append(result, payload...)

	return result, nil
}

// readMessage reads one complete PostgreSQL message (with type byte)
func (fpb *FakePostgreSQLBackend) readMessage(conn net.Conn) ([]byte, error) {
	// Read message type (1 byte)
	typeBuf := make([]byte, 1)
	_, err := conn.Read(typeBuf)
	if err != nil {
		return nil, err
	}

	msgType := typeBuf[0]

	// Read message length (4 bytes, big-endian)
	lenBuf := make([]byte, 4)
	_, err = conn.Read(lenBuf)
	if err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(lenBuf)

	// Read payload (msgLen - 4, since length includes itself)
	if msgLen < 4 {
		return nil, fmt.Errorf("invalid message length: %d", msgLen)
	}

	payload := make([]byte, msgLen-4)
	_, err = io.ReadFull(conn, payload)
	if err != nil {
		return nil, err
	}

	// Reconstruct the full message
	result := []byte{msgType}
	result = append(result, lenBuf...)
	result = append(result, payload...)

	return result, nil
}

// extractQueryString extracts the SQL query from a Query message
// Query message format: 'Q' + length + query_string (null-terminated)
func (fpb *FakePostgreSQLBackend) extractQueryString(msg []byte) string {
	if len(msg) < 6 {
		return ""
	}

	// Skip 'Q' (1 byte) + length field (4 bytes) = 5 bytes
	payload := msg[5:]

	// Find null terminator
	for i, b := range payload {
		if b == 0 {
			return string(payload[:i])
		}
	}

	// No null terminator found
	return strings.TrimSpace(string(payload))
}

// buildAuthenticationOk builds an AuthenticationOk message
func (fpb *FakePostgreSQLBackend) buildAuthenticationOk() []byte {
	msg := []byte{'R'}                        // Authentication response
	msg = append(msg, 0x00, 0x00, 0x00, 0x08) // length: 4 + 4
	msg = append(msg, 0x00, 0x00, 0x00, 0x00) // AuthenticationOk = 0
	return msg
}

// buildParameterStatus builds a ParameterStatus message
func (fpb *FakePostgreSQLBackend) buildParameterStatus(name, value string) []byte {
	// ParameterStatus: 'S' + length + name (null-terminated) + value (null-terminated)
	msg := []byte{'S'}

	body := []byte(name)
	body = append(body, 0)
	body = append(body, []byte(value)...)
	body = append(body, 0)

	length := len(body) + 4 // body + length field
	msg = append(msg, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	msg = append(msg, body...)

	return msg
}

// buildBackendKeyData builds a BackendKeyData message
func (fpb *FakePostgreSQLBackend) buildBackendKeyData(pid, secret uint32) []byte {
	msg := []byte{'K'}                        // BackendKeyData
	msg = append(msg, 0x00, 0x00, 0x00, 0x0C) // length: 4 + 4 + 4

	// Process ID and secret key (both uint32, big-endian)
	msg = append(msg, byte(pid>>24), byte(pid>>16), byte(pid>>8), byte(pid))
	msg = append(msg, byte(secret>>24), byte(secret>>16), byte(secret>>8), byte(secret))

	return msg
}

// buildReadyForQuery builds a ReadyForQuery message
func (fpb *FakePostgreSQLBackend) buildReadyForQuery(txStatus byte) []byte {
	// ReadyForQuery: 'Z' + length + transaction_status
	msg := []byte{'Z'}
	msg = append(msg, 0x00, 0x00, 0x00, 0x05) // length: 4 + 1
	msg = append(msg, txStatus)               // 'I' = idle, 'T' = in transaction, 'E' = error

	return msg
}

// buildCommandComplete builds a CommandComplete message
func (fpb *FakePostgreSQLBackend) buildCommandComplete(tag string) []byte {
	// CommandComplete: 'C' + length + command_tag (null-terminated)
	msg := []byte{'C'}

	body := []byte(tag)
	body = append(body, 0)

	length := len(body) + 4 // body + length field
	msg = append(msg, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	msg = append(msg, body...)

	return msg
}

// buildRowDescription builds a minimal RowDescription message
func (fpb *FakePostgreSQLBackend) buildRowDescription() []byte {
	// RowDescription: 'T' + length + num_fields + field_descriptions
	// For simplicity, just send 1 field (int4)
	msg := []byte{'T'}

	body := []byte{0x00, 0x01} // 1 field

	// Field description: name + oid + size + etc.
	fieldName := "result"
	body = append(body, []byte(fieldName)...)
	body = append(body, 0)
	body = append(body, 0x00, 0x00, 0x00, 0x00) // table oid (0)
	body = append(body, 0x00, 0x00)             // column number (0)
	body = append(body, 0x00, 0x00, 0x00, 0x17) // type oid (23 = int4)
	body = append(body, 0x00, 0x04)             // type size (4)
	body = append(body, 0xFF, 0xFF, 0xFF, 0xFF) // type modifier (-1)
	body = append(body, 0x00, 0x00)             // format code (0 = text)

	length := len(body) + 4
	msg = append(msg, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	msg = append(msg, body...)

	return msg
}

// buildDataRow builds a minimal DataRow message
func (fpb *FakePostgreSQLBackend) buildDataRow() []byte {
	// DataRow: 'D' + length + num_values + value_length + value_data
	msg := []byte{'D'}

	body := []byte{0x00, 0x01} // 1 value
	valueStr := "1"
	body = append(body, byte(len(valueStr)>>8), byte(len(valueStr)))
	body = append(body, []byte(valueStr)...)

	length := len(body) + 4
	msg = append(msg, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	msg = append(msg, body...)

	return msg
}

// buildErrorResponse builds an ErrorResponse message
func (fpb *FakePostgreSQLBackend) buildErrorResponse(message string) []byte {
	// ErrorResponse: 'E' + length + field_pairs (null-terminated)
	msg := []byte{'E'}

	body := []byte{'M'} // Field type 'M' (message)
	body = append(body, []byte(message)...)
	body = append(body, 0)
	body = append(body, 0) // End of error response

	length := len(body) + 4
	msg = append(msg, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	msg = append(msg, body...)

	return msg
}

// Close stops the fake backend server
func (fpb *FakePostgreSQLBackend) Close() {
	close(fpb.done)
	fpb.listener.Close()
}

// GetReceivedQueries returns all queries received
func (fpb *FakePostgreSQLBackend) GetReceivedQueries() []string {
	fpb.mu.Lock()
	defer fpb.mu.Unlock()

	queries := make([]string, len(fpb.ReceivedData))
	for i, data := range fpb.ReceivedData {
		queries[i] = string(data)
	}
	return queries
}
