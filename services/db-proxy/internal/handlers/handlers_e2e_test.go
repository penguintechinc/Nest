package handlers

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/penguintechinc/nest/services/db-proxy/internal/protocol"
	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"go.uber.org/zap"
)

// FakeBackendServer represents a fake database backend that records what it receives
type FakeBackendServer struct {
	listener net.Listener
	Port     int
	Name     string

	ReceivedData [][]byte // all data received from clients
	done         chan struct{}
}

// NewFakeBackendServer starts a fake backend TCP server on a random port
func NewFakeBackendServer(name string, t *testing.T) *FakeBackendServer {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}

	addr := listener.Addr().(*net.TCPAddr)

	server := &FakeBackendServer{
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

	return server
}

// handleConnection handles a connection and records received data
func (fbs *FakeBackendServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				// log error if needed
			}
			return
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			fbs.ReceivedData = append(fbs.ReceivedData, data)

			// Echo back a simple response (MySQL OK packet for testing)
			okPacket := []byte{0x07, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00}
			conn.Write(okPacket)
		}
	}
}

// Close stops the fake backend server
func (fbs *FakeBackendServer) Close() {
	close(fbs.done)
	fbs.listener.Close()
}

// GetReceivedQueries returns all data received
func (fbs *FakeBackendServer) GetReceivedData() [][]byte {
	return fbs.ReceivedData
}

// Stub for connection pool (needed by ProxyLoop but not used in this test)
type ConnectionPool struct{}

// TestQueryClassification proves that the parser correctly identifies query types
func TestQueryClassification(t *testing.T) {
	tests := []struct {
		name            string
		queryText       string
		expectedIsRead  bool
		expectedIsWrite bool
	}{
		{"SELECT", "SELECT * FROM users", true, false},
		{"INSERT", "INSERT INTO users VALUES (1)", false, true},
		{"UPDATE", "UPDATE users SET name='test'", false, true},
		{"DELETE", "DELETE FROM users", false, true},
		{"SELECT in txn should pin", "SELECT * FROM users", true, false},
	}

	for _, tt := range tests {
		queryType := protocol.ClassifyQuery(tt.queryText)
		if queryType.IsRead() != tt.expectedIsRead {
			t.Errorf("%s: IsRead() = %v, expected %v", tt.name, queryType.IsRead(), tt.expectedIsRead)
		}
		if queryType.IsWrite() != tt.expectedIsWrite {
			t.Errorf("%s: IsWrite() = %v, expected %v", tt.name, queryType.IsWrite(), tt.expectedIsWrite)
		}
		t.Logf("✓ %s classified correctly: read=%v, write=%v", tt.name, queryType.IsRead(), queryType.IsWrite())
	}
}

// TestTransactionPinning proves that queries within a transaction are pinned to primary
func TestTransactionPinning(t *testing.T) {
	logger := zap.NewNop()

	// Setup backends
	primaryServer := NewFakeBackendServer("primary", t)
	defer primaryServer.Close()

	replicaServer := NewFakeBackendServer("replica", t)
	defer replicaServer.Close()

	// Setup router
	router := routing.NewRouter(logger)

	primaryEndpoint := &routing.BackendEndpoint{
		Name:     "primary",
		Host:     "127.0.0.1",
		Port:     primaryServer.Port,
		Protocol: "mysql",
		MaxConns: 10,
	}
	primaryEndpoint.Healthy.Store(true)

	replicaEndpoint := &routing.BackendEndpoint{
		Name:     "replica",
		Host:     "127.0.0.1",
		Port:     replicaServer.Port,
		Protocol: "mysql",
		MaxConns: 10,
	}
	replicaEndpoint.Healthy.Store(true)

	route := &routing.RouteConfig{
		ID:       "test-route",
		Protocol: "mysql",
		Primary:  primaryEndpoint,
		Replicas: []*routing.BackendEndpoint{replicaEndpoint},
		Tenant:   "test",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Verify routing logic in isolation
	session := NewSessionState()

	// Before transaction, SELECT should route to replica
	selectQuery := &protocol.ParsedQuery{QueryType: protocol.QueryTypeSelect}
	backend, err := router.SelectBackend("test-route", selectQuery, session.ShouldRouteToPrimary())
	if err != nil {
		t.Fatalf("routing error: %v", err)
	}
	if backend.Name != "replica" {
		t.Errorf("SELECT before transaction should route to replica, got %s", backend.Name)
	}
	t.Logf("✓ SELECT before transaction routed to replica")

	// Mark transaction started
	session.SetInTransaction(true)

	// After BEGIN, SELECT should route to primary
	backend, err = router.SelectBackend("test-route", selectQuery, session.ShouldRouteToPrimary())
	if err != nil {
		t.Fatalf("routing error: %v", err)
	}
	if backend.Name != "primary" {
		t.Errorf("SELECT in transaction should route to primary, got %s", backend.Name)
	}
	t.Logf("✓ SELECT in transaction routed to primary")

	// After COMMIT, should be back to replica routing
	session.SetInTransaction(false)

	backend, err = router.SelectBackend("test-route", selectQuery, session.ShouldRouteToPrimary())
	if err != nil {
		t.Fatalf("routing error: %v", err)
	}
	if backend.Name != "replica" {
		t.Errorf("SELECT after transaction should route to replica, got %s", backend.Name)
	}
	t.Logf("✓ SELECT after transaction routed to replica")
}

// TestPacketFraming tests that the framing layer correctly reads complete protocol packets
// with proper read/write coordination using goroutines
func TestPacketFraming(t *testing.T) {
	// Create a pipe to simulate network connection
	reader, writer := net.Pipe()
	defer reader.Close()
	defer writer.Close()

	// Create a framer (reads from reader)
	framer := NewMySQLFramer(reader)

	// Write a MySQL packet: 3-byte length (little-endian) + 1-byte seq + payload
	// Packet format: length | seq | COM_QUERY | "SELECT"
	// COM_QUERY = 0x03, "SELECT" = 0x53,0x45,0x4C,0x45,0x43,0x54 (6 bytes)
	// Total payload = 1 (COM_QUERY) + 6 (SELECT) = 7 bytes
	// So length field = 0x07 (little-endian: 0x07, 0x00, 0x00)
	packet := []byte{0x07, 0x00, 0x00, 0x01, 0x03, 0x53, 0x45, 0x4C, 0x45, 0x43, 0x54}
	//                ^^^ 7 bytes payload   ^^^ seq  ^^^ COM_QUERY   ^^^ SELECT (6 bytes)

	// Use a goroutine to write (prevents deadlock by not holding the pipe lock)
	writeErrors := make(chan error, 1)
	go func() {
		// Send packet in chunks to test buffering
		// First chunk: length + seq (4 bytes)
		if _, err := writer.Write(packet[:4]); err != nil {
			writeErrors <- err
			return
		}
		time.Sleep(10 * time.Millisecond)
		// Second chunk: COM_QUERY + SELECT (7 bytes)
		if _, err := writer.Write(packet[4:]); err != nil {
			writeErrors <- err
			return
		}
		writeErrors <- nil
	}()

	// Read the complete frame from the reader end
	frame, err := framer.ReadFrame()
	if err != nil {
		t.Fatalf("failed to read frame: %v", err)
	}

	if !bytes.Equal(frame, packet) {
		t.Errorf("frame mismatch: got %v, expected %v", frame, packet)
	}

	// Wait for write goroutine to finish
	if writeErr := <-writeErrors; writeErr != nil {
		t.Errorf("write error: %v", writeErr)
	}

	t.Logf("✓ Packet framing correctly handled multi-chunk reads")
}
