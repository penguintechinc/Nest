package protocol

import (
	"strings"
)

// QueryType represents the classification of a query
type QueryType int

const (
	QueryTypeUnknown QueryType = iota
	QueryTypeSelect
	QueryTypeInsert
	QueryTypeUpdate
	QueryTypeDelete
	QueryTypeDDL
	QueryTypeCall
	QueryTypeBegin
	QueryTypeCommit
	QueryTypeRollback
)

// ParsedQuery represents the parsed structure of a query
type ParsedQuery struct {
	Protocol   string    // "mysql", "postgresql", "redis"
	QueryText  string    // Raw SQL text
	QueryType  QueryType // Read, Write, DDL, etc.
	InTxn      bool      // Whether this query is within an explicit transaction
	IsExtended bool      // PostgreSQL extended protocol (prepared statements) - defer full support
	RawBytes   []byte    // Original protocol bytes for passthrough if parsing fails
	ParseError error     // Error from parsing (still forward if error, log it)
}

// String returns a string representation of the query type
func (qt QueryType) String() string {
	switch qt {
	case QueryTypeSelect:
		return "SELECT"
	case QueryTypeInsert:
		return "INSERT"
	case QueryTypeUpdate:
		return "UPDATE"
	case QueryTypeDelete:
		return "DELETE"
	case QueryTypeDDL:
		return "DDL"
	case QueryTypeCall:
		return "CALL"
	case QueryTypeBegin:
		return "BEGIN"
	case QueryTypeCommit:
		return "COMMIT"
	case QueryTypeRollback:
		return "ROLLBACK"
	default:
		return "UNKNOWN"
	}
}

// IsRead returns true if the query type is a read operation
func (qt QueryType) IsRead() bool {
	return qt == QueryTypeSelect
}

// IsWrite returns true if the query type is a write operation
func (qt QueryType) IsWrite() bool {
	switch qt {
	case QueryTypeInsert, QueryTypeUpdate, QueryTypeDelete, QueryTypeDDL, QueryTypeCall:
		return true
	default:
		return false
	}
}

// IsTxnControl returns true if the query is a transaction control statement
func (qt QueryType) IsTxnControl() bool {
	switch qt {
	case QueryTypeBegin, QueryTypeCommit, QueryTypeRollback:
		return true
	default:
		return false
	}
}

// Parser provides protocol-aware query parsing
type Parser interface {
	Parse(data []byte) (*ParsedQuery, error)
	Protocol() string
}

// NewParser creates a parser for the specified protocol
func NewParser(protocol string) Parser {
	switch strings.ToLower(protocol) {
	case "mysql":
		return &MySQLParser{}
	case "postgresql", "postgres":
		return &PostgreSQLParser{}
	case "redis":
		return &RedisParser{}
	default:
		return &GenericParser{}
	}
}

// GenericParser is a fallback parser for unknown protocols
type GenericParser struct{}

func (g *GenericParser) Protocol() string {
	return "unknown"
}

func (g *GenericParser) Parse(data []byte) (*ParsedQuery, error) {
	// Fail safe: treat as WRITE and don't try to extract query text
	return &ParsedQuery{
		Protocol:  "unknown",
		QueryText: "",
		QueryType: QueryTypeUnknown,
		RawBytes:  data,
	}, nil
}

// TrimAndNormalize normalizes SQL text for classification
func TrimAndNormalize(text string) string {
	// Remove leading/trailing whitespace and comments
	text = strings.TrimSpace(text)

	// Remove leading comments (SQL: -- and /* */)
	for {
		if strings.HasPrefix(text, "--") {
			// Skip line comment
			if idx := strings.Index(text, "\n"); idx != -1 {
				text = strings.TrimSpace(text[idx+1:])
			} else {
				return ""
			}
		} else if strings.HasPrefix(text, "/*") {
			// Skip block comment
			if idx := strings.Index(text, "*/"); idx != -1 {
				text = strings.TrimSpace(text[idx+2:])
			} else {
				return ""
			}
		} else {
			break
		}
	}

	return text
}

// ClassifyQuery determines the query type from the query text
func ClassifyQuery(text string) QueryType {
	text = TrimAndNormalize(text)
	if text == "" {
		return QueryTypeUnknown
	}

	// Extract first keyword
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return QueryTypeUnknown
	}

	keyword := strings.ToUpper(fields[0])

	switch keyword {
	case "SELECT", "WITH":
		// WITH clause can precede a SELECT (CTE)
		if keyword == "WITH" && len(fields) > 1 {
			// WITH ... SELECT is a read
			return QueryTypeSelect
		}
		return QueryTypeSelect
	case "INSERT":
		return QueryTypeInsert
	case "UPDATE":
		return QueryTypeUpdate
	case "DELETE":
		return QueryTypeDelete
	case "CREATE", "ALTER", "DROP", "TRUNCATE":
		return QueryTypeDDL
	case "CALL":
		return QueryTypeCall
	case "BEGIN", "START":
		if len(fields) > 1 && strings.ToUpper(fields[1]) == "TRANSACTION" {
			return QueryTypeBegin
		}
		if keyword == "BEGIN" {
			return QueryTypeBegin
		}
		return QueryTypeUnknown
	case "COMMIT":
		return QueryTypeCommit
	case "ROLLBACK":
		return QueryTypeRollback
	case "SHOW", "EXPLAIN", "DESCRIBE":
		// These are read-like, but not strictly SELECT
		return QueryTypeSelect
	default:
		return QueryTypeUnknown
	}
}

// IsSessionDirtyingStatement checks if a query dirties the session state
// (requires routing to primary for all subsequent queries)
// These include: SET, PREPARE, DECLARE, LISTEN, CREATE TEMP TABLE, etc.
func IsSessionDirtyingStatement(text string) bool {
	text = TrimAndNormalize(text)
	if text == "" {
		return false
	}

	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}

	keyword := strings.ToUpper(fields[0])

	// Set statements dirty the session
	if keyword == "SET" {
		return true
	}

	// Prepare statement (PostgreSQL extended protocol)
	if keyword == "PREPARE" {
		return true
	}

	// Declare cursor (PostgreSQL)
	if keyword == "DECLARE" {
		return true
	}

	// Listen (PostgreSQL pub/sub)
	if keyword == "LISTEN" || keyword == "UNLISTEN" {
		return true
	}

	// Create temporary table
	if keyword == "CREATE" && len(fields) > 1 {
		secondKeyword := strings.ToUpper(fields[1])
		if secondKeyword == "TEMP" || secondKeyword == "TEMPORARY" {
			return true
		}
	}

	// Pragma (SQLite)
	if keyword == "PRAGMA" {
		return true
	}

	// Use (MySQL - but routed as DDL)
	if keyword == "USE" {
		return true
	}

	return false
}
