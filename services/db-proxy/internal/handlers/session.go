package handlers

import (
	"net"
	"sync"

	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
)

// SessionState tracks the state of a client session
type SessionState struct {
	mu sync.Mutex

	// primary connection is always available and owns session state
	primaryConn *BackendConnection

	// replica connection for reads (reused, not per-backend)
	replicaConn *BackendConnection

	// flags
	inTransaction bool   // inside explicit BEGIN/COMMIT/ROLLBACK block
	stateDirty    bool   // session has issued USE, SET, prepared statements, etc.
	lastUsedRoute string // last route used (for connection reuse)
}

// BackendConnection represents a connection to a backend (primary or replica)
type BackendConnection struct {
	conn      net.Conn // underlying network connection
	framer    ProtocolFramer
	endpoint  *routing.BackendEndpoint
	isReplica bool
}

// SetInTransaction marks whether we're in an explicit transaction
func (ss *SessionState) SetInTransaction(inTxn bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.inTransaction = inTxn
}

// IsInTransaction returns whether we're in an explicit transaction
func (ss *SessionState) IsInTransaction() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.inTransaction
}

// MarkStateDirty marks the session as having mutable state (USE, SET, prepared statement, etc.)
func (ss *SessionState) MarkStateDirty() {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.stateDirty = true
}

// IsStateDirty returns whether the session has mutable state
func (ss *SessionState) IsStateDirty() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.stateDirty
}

// ShouldRouteToPrimary returns true if we must route to primary (transaction, dirty state, or unknown)
func (ss *SessionState) ShouldRouteToPrimary() bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.inTransaction || ss.stateDirty
}

// SetPrimaryConnection stores the primary backend connection
func (ss *SessionState) SetPrimaryConnection(conn *BackendConnection) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.primaryConn = conn
}

// GetPrimaryConnection returns the primary connection
func (ss *SessionState) GetPrimaryConnection() *BackendConnection {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.primaryConn
}

// SetReplicaConnection stores the replica connection
func (ss *SessionState) SetReplicaConnection(conn *BackendConnection) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.replicaConn = conn
}

// GetReplicaConnection returns the replica connection (may be nil)
func (ss *SessionState) GetReplicaConnection() *BackendConnection {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.replicaConn
}

// NewSessionState creates a new session state
func NewSessionState() *SessionState {
	return &SessionState{
		inTransaction: false,
		stateDirty:    false,
	}
}
