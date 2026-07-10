package cache

import (
	"regexp"
	"strings"
)

var (
	// Regex to normalize whitespace
	whitespaceRegex = regexp.MustCompile(`\s+`)

	// Regex to detect non-deterministic functions
	nonDeterministicFuncs = []string{
		"NOW", "CURRENT_TIMESTAMP", "CURRENT_DATE", "CURRENT_TIME",
		"LOCALTIME", "LOCALTIMESTAMP",
		"RAND", "RANDOM",
		"UUID", "UUID_GENERATE", "NEWID", "GEN_RANDOM_UUID",
		"NEXTVAL", "NEXT_VALUE",
		"SEQUENCE_NEXTVAL",
		"LASTVAL", "LASTINSERTID", "LAST_INSERT_ID",
		"UNIX_TIMESTAMP", "CURRENT_USER", "SESSION_USER",
		"ROW_NUMBER", "RANK", "DENSE_RANK", "LAG", "LEAD",
	}

	// Precompiled regex for detecting FOR UPDATE/SHARE
	forUpdateRegex = regexp.MustCompile(`\bFOR\s+(UPDATE|SHARE)\b`)

	// Precompiled regexes for function detection
	functionRegexes = make(map[string]*regexp.Regexp)
)

func init() {
	for _, fn := range nonDeterministicFuncs {
		// Create regex that matches function call: FUNCTION( or FUNCTION (
		pattern := `\b` + fn + `\s*\(`
		functionRegexes[fn] = regexp.MustCompile(pattern)
	}
}

// normalizeQuery normalizes SQL query text for consistent cache key generation
// This removes:
// - Leading/trailing whitespace
// - Excess whitespace between tokens
// - SQL comments
// - Case variations (normalized to uppercase)
func normalizeQuery(queryText string) string {
	// Remove leading/trailing whitespace
	text := strings.TrimSpace(queryText)

	// Remove SQL line comments (-- ...)
	text = removeLineComments(text)

	// Remove SQL block comments (/* ... */)
	text = removeBlockComments(text)

	// Normalize whitespace: collapse multiple spaces to single space
	text = whitespaceRegex.ReplaceAllString(text, " ")

	// Trim again after comment removal
	text = strings.TrimSpace(text)

	// Normalize to uppercase for comparison consistency
	text = strings.ToUpper(text)

	return text
}

// removeLineComments removes SQL line comments (-- ...)
func removeLineComments(text string) string {
	var result strings.Builder

	for {
		idx := strings.Index(text, "--")
		if idx == -1 {
			result.WriteString(text)
			break
		}

		// Write text before comment
		result.WriteString(text[:idx])

		// Find end of line
		endIdx := strings.Index(text[idx:], "\n")
		if endIdx == -1 {
			// No newline, comment extends to end
			break
		}

		// Skip to after the newline
		text = text[idx+endIdx+1:]
	}

	return result.String()
}

// removeBlockComments removes SQL block comments (/* ... */)
func removeBlockComments(text string) string {
	for {
		startIdx := strings.Index(text, "/*")
		if startIdx == -1 {
			break
		}

		endIdx := strings.Index(text[startIdx:], "*/")
		if endIdx == -1 {
			// Unterminated comment, remove to end
			text = text[:startIdx]
			break
		}

		// Remove the comment
		text = text[:startIdx] + text[startIdx+endIdx+2:]
	}

	return text
}

// IsCacheable determines if a query is safe to cache
// A query is cacheable if:
// 1. It's a SELECT query
// 2. It doesn't contain FOR UPDATE or FOR SHARE
// 3. It doesn't contain non-deterministic functions
// 4. Not in a transaction
// 5. Session not dirty
func IsCacheable(queryType int, queryText string, inTransaction bool, stateDirty bool) bool {
	// Only cache SELECTs
	if queryType != 1 { // QueryTypeSelect = 1
		return false
	}

	// Don't cache if in transaction
	if inTransaction {
		return false
	}

	// Don't cache if session is dirty
	if stateDirty {
		return false
	}

	normalized := normalizeQuery(queryText)

	// Check for FOR UPDATE or FOR SHARE
	if forUpdateRegex.MatchString(normalized) {
		return false
	}

	// Check for non-deterministic functions
	for fn, regex := range functionRegexes {
		if regex.MatchString(normalized) {
			return false
		}
		_ = fn // silence unused warning
	}

	return true
}

// ExtractTablesFromQuery extracts table names from a query for cache invalidation
// This is a best-effort approach - complex queries may not be fully parsed
// For SELECTs, extracts the tables being read (so invalidation knows what to clear)
// For writes, extracts the tables being modified (so other SELECT caches can be invalidated)
func ExtractTablesFromQuery(queryType int, queryText string) []string {
	normalized := normalizeQuery(queryText)

	var tables []string

	// For SELECT: extract FROM/JOIN table names (so we know what tables this cached result depends on)
	if queryType == 1 { // QueryTypeSelect
		tables = append(tables, extractTablesFromSelect(normalized)...)
	}

	// For INSERT: INSERT INTO table_name
	if queryType == 2 { // QueryTypeInsert
		if table := extractTableFromInsert(normalized); table != "" {
			tables = append(tables, table)
		}
	}

	// For UPDATE: UPDATE table_name SET ...
	if queryType == 3 { // QueryTypeUpdate
		if table := extractTableFromUpdate(normalized); table != "" {
			tables = append(tables, table)
		}
	}

	// For DELETE: DELETE FROM table_name
	if queryType == 4 { // QueryTypeDelete
		if table := extractTableFromDelete(normalized); table != "" {
			tables = append(tables, table)
		}
	}

	// For DDL: TRUNCATE, ALTER TABLE, etc.
	if queryType == 5 { // QueryTypeDDL
		tables = append(tables, extractTablesFromDDL(normalized)...)
	}

	return tables
}

// extractTablesFromSelect extracts table names from a SELECT query
func extractTablesFromSelect(query string) []string {
	var tables []string
	parts := strings.Fields(query)

	for i, part := range parts {
		if part == "FROM" && i+1 < len(parts) {
			// Simple case: FROM table_name
			tables = append(tables, cleanTableName(parts[i+1]))
			break
		}
	}

	// TODO: handle JOINs if needed (for now, simple FROM is good enough)
	return tables
}

// extractTableFromInsert extracts table name from INSERT query
func extractTableFromInsert(query string) string {
	// Pattern: INSERT INTO table_name ...
	parts := strings.Fields(query)
	for i, part := range parts {
		if part == "INSERT" && i+2 < len(parts) && parts[i+1] == "INTO" {
			return cleanTableName(parts[i+2])
		}
	}
	return ""
}

// extractTableFromUpdate extracts table name from UPDATE query
func extractTableFromUpdate(query string) string {
	// Pattern: UPDATE table_name SET ...
	parts := strings.Fields(query)
	for i, part := range parts {
		if part == "UPDATE" && i+1 < len(parts) {
			return cleanTableName(parts[i+1])
		}
	}
	return ""
}

// extractTableFromDelete extracts table name from DELETE query
func extractTableFromDelete(query string) string {
	// Pattern: DELETE FROM table_name ...
	parts := strings.Fields(query)
	for i, part := range parts {
		if part == "DELETE" && i+2 < len(parts) && parts[i+1] == "FROM" {
			return cleanTableName(parts[i+2])
		}
	}
	return ""
}

// extractTablesFromDDL extracts table names from DDL query
func extractTablesFromDDL(query string) []string {
	var tables []string
	parts := strings.Fields(query)

	for i, part := range parts {
		switch part {
		case "CREATE", "ALTER", "DROP", "TRUNCATE":
			// Look for TABLE keyword
			if i+1 < len(parts) {
				if parts[i+1] == "TABLE" && i+2 < len(parts) {
					tables = append(tables, cleanTableName(parts[i+2]))
				}
			}
		}
	}

	return tables
}

// cleanTableName removes schema prefix and backticks/quotes from table name
func cleanTableName(name string) string {
	// Remove backticks or quotes
	name = strings.Trim(name, "`\"'[]")

	// Remove schema prefix (schema.table -> table)
	if idx := strings.LastIndex(name, "."); idx != -1 {
		name = name[idx+1:]
	}

	// Remove any trailing punctuation
	name = strings.TrimRight(name, ",();")

	return name
}
