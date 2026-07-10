package protocol

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
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
	backendUser     string // username sent in the StartupMessage (needed for MD5)
	backendPassword string // password to use for auth (if needed)

	// nonceGen produces the SCRAM client nonce. Overridable in tests for
	// deterministic exchanges; defaults to a crypto/rand-backed generator.
	nonceGen func() (string, error)

	// SCRAM-SHA-256 exchange state (populated across the SASL round trips).
	scramClientFirstBare string // "n=<user>,r=<client-nonce>"
	scramServerSignature []byte // expected server signature, verified on SASLFinal
}

// NewAuthHandler creates a new auth handler for the given backend credentials.
func NewAuthHandler(user, password string) *AuthHandler {
	return &AuthHandler{
		backendUser:     user,
		backendPassword: password,
		nonceGen:        defaultSCRAMNonce,
	}
}

// defaultSCRAMNonce returns a 24-byte base64 client nonce from crypto/rand.
func defaultSCRAMNonce() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate SCRAM nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// HandleStartup handles the authentication exchange after sending StartupMessage
// Reads and processes auth messages until ReadyForQuery or error
// Returns error if auth fails
func (ah *AuthHandler) HandleStartup(conn io.ReadWriter) error {
	// Read and process auth flow until ReadyForQuery
	for {
		// Read message type (1 byte). io.ReadFull guards against short reads
		// (TCP may deliver a header split across segments).
		msgTypeBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, msgTypeBuf); err != nil {
			return fmt.Errorf("failed to read message type: %w", err)
		}
		msgType := msgTypeBuf[0]

		// Read message length (4 bytes, big-endian, includes the 4 bytes themselves)
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return fmt.Errorf("failed to read message length: %w", err)
		}
		msgLen := binary.BigEndian.Uint32(lenBuf)
		if msgLen < 4 {
			return fmt.Errorf("invalid message length: %d", msgLen)
		}

		// Read the payload (msgLen - 4, since length includes itself)
		payload := make([]byte, msgLen-4)
		if _, err := io.ReadFull(conn, payload); err != nil {
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
		md5pw := ah.md5Password(ah.backendUser, ah.backendPassword, salt)
		pwMsg := ah.passwordMessage(md5pw)
		_, err := conn.Write(pwMsg)
		return err

	case BackendAuthSASL:
		// SASL authentication: the payload lists the mechanisms the backend
		// offers. We support SCRAM-SHA-256 (non channel-binding). CNPG and
		// modern PostgreSQL default to this.
		return ah.handleSASLInit(conn, payload[4:])

	case BackendAuthSASLContinue:
		// server-first-message: compute and send the client-final-message.
		return ah.handleSASLContinue(conn, payload[4:])

	case BackendAuthSASLFinal:
		// server-final-message: verify the server signature.
		return ah.handleSASLFinal(payload[4:])

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

// md5Password constructs the MD5 password token PostgreSQL expects:
//
//	"md5" + hex( md5( hex( md5(password + username) ) + salt ) )
//
// The username is part of the inner hash — omitting it (as the previous
// implementation did) produces a token the backend always rejects.
func (ah *AuthHandler) md5Password(username, password string, salt []byte) string {
	inner := md5.Sum([]byte(password + username))
	innerHex := fmt.Sprintf("%x", inner)
	outer := md5.Sum(append([]byte(innerHex), salt...))
	return "md5" + fmt.Sprintf("%x", outer)
}

// ── SCRAM-SHA-256 (RFC 5802 / RFC 7677), proxy acting as the client ──────────

const scramMechSHA256 = "SCRAM-SHA-256"

// gs2NoBinding is the gs2-header for "no channel binding", base64 of "n,,".
const gs2NoBindingB64 = "biws"

// handleSASLInit processes an AuthenticationSASL message. mechList is a
// sequence of null-terminated mechanism names, terminated by an empty string.
// It selects SCRAM-SHA-256 and sends the SASLInitialResponse.
func (ah *AuthHandler) handleSASLInit(conn io.Writer, mechList []byte) error {
	if ah.backendPassword == "" {
		return fmt.Errorf("backend requires SASL auth but no password configured")
	}

	offered := false
	for _, mech := range strings.Split(string(mechList), "\x00") {
		if mech == scramMechSHA256 {
			offered = true
			break
		}
	}
	if !offered {
		return fmt.Errorf("backend does not offer %s (SCRAM-SHA-256-PLUS/channel binding not supported)", scramMechSHA256)
	}

	if ah.nonceGen == nil {
		ah.nonceGen = defaultSCRAMNonce
	}
	clientNonce, err := ah.nonceGen()
	if err != nil {
		return err
	}

	// PostgreSQL takes the username from the StartupMessage, so the SCRAM
	// n= field is left empty.
	ah.scramClientFirstBare = "n=,r=" + clientNonce
	clientFirst := "n,," + ah.scramClientFirstBare

	// SASLInitialResponse: 'p' + len + mechanism(null-terminated) +
	// int32(client-first length) + client-first-message.
	var body []byte
	body = append(body, []byte(scramMechSHA256)...)
	body = append(body, 0)
	body = binary.BigEndian.AppendUint32(body, uint32(len(clientFirst)))
	body = append(body, []byte(clientFirst)...)

	_, err = conn.Write(taggedMessage('p', body))
	return err
}

// handleSASLContinue processes the server-first-message and replies with the
// client-final-message carrying the ClientProof.
func (ah *AuthHandler) handleSASLContinue(conn io.Writer, serverFirst []byte) error {
	if ah.scramClientFirstBare == "" {
		return fmt.Errorf("received SASLContinue before SASLInitialResponse")
	}
	serverFirstStr := string(serverFirst)

	combinedNonce, saltB64, iterStr, err := parseSCRAMServerFirst(serverFirstStr)
	if err != nil {
		return err
	}
	// The server nonce must extend our client nonce.
	clientNonce := strings.TrimPrefix(ah.scramClientFirstBare, "n=,r=")
	if !strings.HasPrefix(combinedNonce, clientNonce) || combinedNonce == clientNonce {
		return fmt.Errorf("SCRAM server nonce does not extend client nonce")
	}
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return fmt.Errorf("invalid SCRAM salt: %w", err)
	}
	iterations, err := strconv.Atoi(iterStr)
	if err != nil || iterations <= 0 {
		return fmt.Errorf("invalid SCRAM iteration count %q", iterStr)
	}

	clientFinalNoProof := "c=" + gs2NoBindingB64 + ",r=" + combinedNonce
	authMessage := ah.scramClientFirstBare + "," + serverFirstStr + "," + clientFinalNoProof

	saltedPassword := pbkdf2SHA256([]byte(ah.backendPassword), salt, iterations, sha256.Size)
	clientKey := hmacSHA256(saltedPassword, []byte("Client Key"))
	storedKey := sha256.Sum256(clientKey)
	clientSig := hmacSHA256(storedKey[:], []byte(authMessage))
	clientProof := xorBytes(clientKey, clientSig)

	// Remember the expected server signature to verify on SASLFinal.
	serverKey := hmacSHA256(saltedPassword, []byte("Server Key"))
	ah.scramServerSignature = hmacSHA256(serverKey, []byte(authMessage))

	clientFinal := clientFinalNoProof + ",p=" + base64.StdEncoding.EncodeToString(clientProof)
	_, err = conn.Write(taggedMessage('p', []byte(clientFinal)))
	return err
}

// handleSASLFinal verifies the server-final-message signature (server proves it
// knows the stored credentials — defends against a MITM backend).
func (ah *AuthHandler) handleSASLFinal(serverFinal []byte) error {
	if ah.scramServerSignature == nil {
		return fmt.Errorf("received SASLFinal before client-final-message")
	}
	for _, attr := range strings.Split(string(serverFinal), ",") {
		switch {
		case strings.HasPrefix(attr, "v="):
			got, err := base64.StdEncoding.DecodeString(attr[2:])
			if err != nil {
				return fmt.Errorf("invalid SCRAM server signature: %w", err)
			}
			if subtle.ConstantTimeCompare(got, ah.scramServerSignature) != 1 {
				return fmt.Errorf("SCRAM server signature mismatch — backend authentication cannot be trusted")
			}
			return nil
		case strings.HasPrefix(attr, "e="):
			return fmt.Errorf("SCRAM auth failed: %s", attr[2:])
		}
	}
	return fmt.Errorf("SCRAM server-final-message missing verifier")
}

// parseSCRAMServerFirst extracts r=, s=, i= from the server-first-message.
func parseSCRAMServerFirst(msg string) (nonce, salt, iter string, err error) {
	for _, attr := range strings.Split(msg, ",") {
		if len(attr) < 2 || attr[1] != '=' {
			continue
		}
		switch attr[0] {
		case 'r':
			nonce = attr[2:]
		case 's':
			salt = attr[2:]
		case 'i':
			iter = attr[2:]
		case 'e':
			return "", "", "", fmt.Errorf("SCRAM auth error: %s", attr[2:])
		}
	}
	if nonce == "" || salt == "" || iter == "" {
		return "", "", "", fmt.Errorf("malformed SCRAM server-first-message")
	}
	return nonce, salt, iter, nil
}

// taggedMessage frames a body as a typed PostgreSQL message: type + int32 length
// (length includes the 4 length bytes) + body.
func taggedMessage(msgType byte, body []byte) []byte {
	msg := []byte{msgType}
	msg = binary.BigEndian.AppendUint32(msg, uint32(len(body)+4))
	return append(msg, body...)
}

// hmacSHA256 returns HMAC-SHA256(key, msg).
func hmacSHA256(key, msg []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return mac.Sum(nil)
}

// pbkdf2SHA256 implements PBKDF2 with HMAC-SHA256 (RFC 8018) — stdlib only, so
// no golang.org/x/crypto dependency is pulled in.
func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	const hLen = sha256.Size
	numBlocks := (keyLen + hLen - 1) / hLen
	var out []byte
	for block := 1; block <= numBlocks; block++ {
		// U1 = HMAC(password, salt || INT_32_BE(block))
		var blockIdx [4]byte
		binary.BigEndian.PutUint32(blockIdx[:], uint32(block))
		u := hmacSHA256(password, append(append([]byte{}, salt...), blockIdx[:]...))
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iterations; i++ {
			u = hmacSHA256(password, u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

// xorBytes returns a XOR b (a and b must be equal length).
func xorBytes(a, b []byte) []byte {
	out := make([]byte, len(a))
	for i := range a {
		out[i] = a[i] ^ b[i]
	}
	return out
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
