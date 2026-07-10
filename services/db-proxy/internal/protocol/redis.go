package protocol

// RedisParser parses Redis protocol messages
// Redis uses RESP (Redis Serialization Protocol) which is command-line based
// We don't need deep parsing here; just recognize commands as READ or WRITE
type RedisParser struct{}

func (r *RedisParser) Protocol() string {
	return "redis"
}

// Parse extracts the Redis command from RESP data
// RESP array format: *<number of arguments>\r\n$<length>\r\n<data>\r\n...
// Redis commands: GET, SET, DEL, INCR, LPUSH, RPUSH, SADD, etc.
// For now, we route based on command name recognition.
func (r *RedisParser) Parse(data []byte) (*ParsedQuery, error) {
	// Redis protocol parsing is minimal for this MVP
	// Just return the raw data; security checker handles command-level filtering
	// Full routing for Redis is deferred (focus on SQL databases first)

	return &ParsedQuery{
		Protocol:  "redis",
		QueryText: string(data),
		QueryType: QueryTypeUnknown, // Redis doesn't fit the SELECT/INSERT model
		RawBytes:  data,
	}, nil
}
