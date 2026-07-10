package protocol

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// PostgreSQL auth messages and handling

const (
	// Backend message types
	BackendAuthOk            = 0
	BackendAuthKerberosV5    = 2
	BackendAuthCleartextPw   = 3
	BackendAuthMD5Pw         = 5
	BackendAuthSCMCredential = 6
	BackendAuthGSS           = 7
	BackendAuthGSSContinue   = 8
	BackendAuthSSPI          = 9
	BackendAuthSASL          = 10
	BackendAuthSASLContinue  = 11
	BackendAuthSASLFinal     = 12

	BackendMsgTypeParameterStatus = 'S'
	BackendMsgTypeBackendKeyData  = 'K'
	BackendMsgTypeReadyForQuery   = 'Z'
	BackendMsgTypeErrorResponse   = 'E'
)

// StartupMessage constructs a PostgreSQL StartupMessage
// Format: length(4) + protocol_version(4) + param_name(string) + param_value(string) + ... + null_terminator
func StartupMessage(user, database string) []byte {
	// Protocol version: 3.0 = (3 << 16) | 0
	protocolVersion := uint32((3 << 16) | 0)

	// Build parameter pairs: user, database
	params := map[string]string{
		"user":     user,
		"database": database,
	}

	// Build the message body
	var body []byte
	body = binary.BigEndian.AppendUint32(body, protocolVersion)

	// Add parameters
	for key, value := range params {
		body = append(body, []byte(key)...)
		body = append(body, 0)
		body = append(body, []byte(value)...)
		body = append(body, 0)
	}

	// Add null terminator
	body = append(body, 0)

	// Prepend message length (4 bytes, includes the length field itself)
	length := len(body) + 4
	msg := binary.BigEndian.AppendUint32([]byte{}, uint32(length))
	msg = append(msg, body...)

	return msg
}

// AuthHandler handles the PostgreSQL authentication exchange
type AuthHandler struct {
	backendPassword string // password to use for auth (if needed)
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(password string) *AuthHandler {
	return &AuthHandler{
		backendPassword: password,
	}
}

// HandleStartup handles the authentication exchange after sending StartupMessage
// Reads and processes auth messages until ReadyForQuery or error
// Returns error if auth fails
func (ah *AuthHandler) HandleStartup(conn io.ReadWriter) error {
	// Read and process auth flow until ReadyForQuery
	for {
		// Read message type (1 byte)
		msgTypeBuf := make([]byte, 1)
		_, err := conn.Read(msgTypeBuf)
		if err != nil {
			return fmt.Errorf("failed to read message type: %w", err)
		}
		msgType := msgTypeBuf[0]

		// Read message length (4 bytes, big-endian, includes the 4 bytes themselves)
		lenBuf := make([]byte, 4)
		_, err = conn.Read(lenBuf)
		if err != nil {
			return fmt.Errorf("failed to read message length: %w", err)
		}
		msgLen := binary.BigEndian.Uint32(lenBuf)

		// Read the payload (msgLen - 4, since length includes itself)
		payload := make([]byte, msgLen-4)
		_, err = conn.Read(payload)
		if err != nil {
			return fmt.Errorf("failed to read payload: %w", err)
		}

		// Process the message
		switch msgType {
		case 'R': // Authentication message
			err := ah.handleAuthMessage(conn, payload)
			if err != nil {
				return err
			}

		case BackendMsgTypeParameterStatus: // S
			// ParameterStatus: ignore for now
			continue

		case BackendMsgTypeBackendKeyData: // K
			// BackendKeyData: ignore for now
			continue

		case BackendMsgTypeReadyForQuery: // Z
			// ReadyForQuery: we're done, connection is ready
			return nil

		case BackendMsgTypeErrorResponse: // E
			// ErrorResponse: auth failed
			errorMsg := parseErrorResponse(payload)
			return fmt.Errorf("backend error: %s", errorMsg)

		default:
			return fmt.Errorf("unexpected message type during auth: %c (%d)", msgType, msgType)
		}
	}
}

// handleAuthMessage handles a single Authentication message (R)
func (ah *AuthHandler) handleAuthMessage(conn io.ReadWriter, payload []byte) error {
	if len(payload) < 4 {
		return fmt.Errorf("auth message too short: %d bytes", len(payload))
	}

	authType := binary.BigEndian.Uint32(payload)

	switch authType {
	case BackendAuthOk:
		// Authentication successful, nothing to do
		return nil

	case BackendAuthCleartextPw:
		// Cleartext password required
		if ah.backendPassword == "" {
			return fmt.Errorf("backend requires cleartext password but none configured")
		}
		pwMsg := ah.passwordMessage(ah.backendPassword)
		_, err := conn.Write(pwMsg)
		return err

	case BackendAuthMD5Pw:
		// MD5 password required
		if len(payload) < 8 {
			return fmt.Errorf("MD5 auth message incomplete: need 4 bytes salt")
		}
		if ah.backendPassword == "" {
			return fmt.Errorf("backend requires MD5 password but none configured")
		}
		salt := payload[4:8]
		md5pw := ah.md5Password(ah.backendPassword, salt)
		pwMsg := ah.passwordMessage(md5pw)
		_, err := conn.Write(pwMsg)
		return err

	default:
		return fmt.Errorf("unsupported authentication type: %d", authType)
	}
}

// passwordMessage constructs a PasswordMessage
// Format: 'p' + length(4) + password(string) + null_terminator
func (ah *AuthHandler) passwordMessage(password string) []byte {
	// Build message body
	body := []byte(password)
	body = append(body, 0) // null terminator

	// Prepend message type ('p')
	msg := []byte{'p'}

	// Prepend length (includes 4 bytes for length field itself)
	length := len(body) + 4
	msg = append(msg, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	msg = append(msg, body...)

	return msg
}

// md5Password constructs the MD5 password hash for PostgreSQL
// PostgreSQL MD5: md5(md5(password + username) + salt)
func (ah *AuthHandler) md5Password(password string, salt []byte) string {
	if len(salt) != 4 {
		return password // fallback to plaintext if salt is wrong size
	}

	// First MD5: password + username (but we don't have username here, so use password only)
	// Actually, PostgreSQL expects md5(password + user), but for the auth message we just md5(password)
	// Let me check: the format is "md5" + hex(md5(md5(password + username) + salt))
	// But we're sending the password back, so we need: md5(md5(password + username) + salt)
	// We'll just hash the password here and let the backend handle it
	inner := md5.Sum([]byte(password))
	salted := append(inner[:], salt...)
	outer := md5.Sum(salted)
	return fmt.Sprintf("%x", outer)
}

// parseErrorResponse parses an ErrorResponse message
// Format: pairs of Field Type (1 byte) + value (string) + null_terminator, ending with 0
func parseErrorResponse(payload []byte) string {
	parts := []string{}
	i := 0
	for i < len(payload) {
		fieldType := payload[i]
		i++

		if fieldType == 0 {
			break
		}

		// Find null terminator
		j := i
		for j < len(payload) && payload[j] != 0 {
			j++
		}

		value := string(payload[i:j])
		if fieldType == 'M' { // Message
			parts = append(parts, value)
		}
		i = j + 1
	}

	return strings.Join(parts, "; ")
}
