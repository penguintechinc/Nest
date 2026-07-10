# nest DB Proxy

Database TCP proxy with dynamic configuration, security inspection, and rate limiting.

## Features

- **Multi-protocol support**: MySQL, PostgreSQL, Redis, MongoDB, MSSQL (extensible)
- **Security inspection**: SQL injection detection on live path (regex-based heuristics)
- **Connection pooling**: Per-protocol connection management
- **Rate limiting**: Connection and query rate limits per protocol
- **Dynamic configuration**: Redis-backed, hot-reloadable routes and security policies
- **gRPC config API**: `ConfigService` for querying and updating configuration
- **Prometheus metrics**: Connection stats, query counts, blocked queries
- **XDP/AF_XDP support**: Compile-time flag + runtime capability detection with fallback
- **Rootless**: Runs as unprivileged user (UID 1000)

## Building

### Standard build (no XDP)
```bash
go build -o db-proxy ./cmd/main.go
```

### XDP-enabled build
```bash
go build -tags xdp -o db-proxy ./cmd/main.go
```

### Docker (default: XDP enabled)
```bash
docker build -t db-proxy:latest .
docker build --build-arg XDP_BUILD_TAGS=noxdp -t db-proxy:no-xdp .
```

### Multi-arch build
```bash
docker buildx build --platform linux/amd64,linux/arm64 -t db-proxy:latest .
```

## Running

### Local
```bash
export DBPROXY_LISTEN_PORT=5432
export DBPROXY_REDIS_ADDR=localhost
export DBPROXY_REDIS_PORT=6379
export DBPROXY_XDP_ENABLED=false  # or true if NET_ADMIN available
./db-proxy
```

### Docker
```bash
docker run -it --rm \
  -p 5432:5432 \
  -p 9090:9090 \
  -e DBPROXY_REDIS_ADDR=host.docker.internal \
  db-proxy:latest
```

### With XDP (requires NET_ADMIN capability)
```bash
docker run -it --rm \
  --cap-add=NET_ADMIN \
  --cap-add=NET_RAW \
  -p 5432:5432 \
  -p 9090:9090 \
  -e DBPROXY_XDP_ENABLED=true \
  db-proxy:latest
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DBPROXY_LISTEN_ADDR` | `0.0.0.0` | Proxy listen address |
| `DBPROXY_LISTEN_PORT` | `5432` | Base proxy port (protocols offset: MySQL+0, PostgreSQL+1, Redis+2) |
| `DBPROXY_GRPC_ADDR` | `0.0.0.0` | gRPC config API address |
| `DBPROXY_GRPC_PORT` | `50051` | gRPC config API port |
| `DBPROXY_METRICS_ADDR` | `0.0.0.0:9090` | Metrics/health server address |
| `DBPROXY_MAX_CONNS_PER_ROUTE` | `10` | Max connections per route |
| `DBPROXY_MAX_CONNS_PER_HOST` | `100` | Max connections per backend host |
| `DBPROXY_CONN_RATE_LIMIT` | `100` | Connection rate limit (per second) |
| `DBPROXY_QUERY_RATE_LIMIT` | `1000` | Query rate limit (per second) |
| `DBPROXY_REDIS_ADDR` | `localhost` | Redis address for config watch |
| `DBPROXY_REDIS_PORT` | `6379` | Redis port |
| `DBPROXY_REDIS_DB` | `0` | Redis DB index |
| `DBPROXY_REDIS_PREFIX` | `nest:dbproxy` | Redis key prefix for config |
| `DBPROXY_XDP_ENABLED` | `false` | Enable XDP fast path (requires NET_ADMIN) |
| `DEBUG` | `false` | Enable debug logging |

### Configuration Schema

Routes and security policies are stored in Redis and can be reloaded dynamically. See [CONFIG_CONTRACT.md](CONFIG_CONTRACT.md) for the complete schema and gRPC API documentation.

**Example route configuration:**
```bash
redis-cli SET nest:dbproxy:routes '{
  "routes": {
    "db-primary": {
      "protocol": "mysql",
      "backend": "db.internal",
      "port": 3306,
      "max_connections": 20,
      "tenant": "tenant-1"
    }
  }
}'
redis-cli PUBLISH nest:dbproxy:config:updated "routes"
```

**Example security configuration:**
```bash
redis-cli SET nest:dbproxy:security '{
  "blocked_resources": ["DROP TABLE", "TRUNCATE"],
  "allowed_resources": [],
  "enable_injection_check": true
}'
redis-cli PUBLISH nest:dbproxy:config:updated "security"
```

## Health Checks

### Liveness (HTTP)
```bash
curl http://localhost:9090/healthz
```

### Readiness (HTTP)
```bash
curl http://localhost:9090/ready
```

### Status (HTTP)
```bash
curl http://localhost:9090/status
```

### Metrics (Prometheus)
```bash
curl http://localhost:9090/metrics
```

## gRPC Config API

Port: `50051` (default)

### Get configuration
```bash
grpcurl -plaintext \
  -d '{"api_version": "v1", "config_key": "routes"}' \
  localhost:50051 \
  nest.dbproxy.v1.ConfigService/GetConfig
```

### Set configuration
```bash
grpcurl -plaintext \
  -d '{"api_version": "v1", "config_key": "routes", "config_data": "..."}' \
  localhost:50051 \
  nest.dbproxy.v1.ConfigService/SetConfig
```

### Reload configuration
```bash
grpcurl -plaintext \
  -d '{"api_version": "v1"}' \
  localhost:50051 \
  nest.dbproxy.v1.ConfigService/ReloadConfig
```

### Get status
```bash
grpcurl -plaintext \
  -d '{"api_version": "v1"}' \
  localhost:50051 \
  nest.dbproxy.v1.ConfigService/GetStatus
```

## Network Modes

### Standard Mode
- Uses standard Go `net` package
- No special capabilities required
- Slightly higher latency than XDP mode

### XDP Mode
- Compile-time flag: `-tags xdp`
- Runtime detection: checks NET_ADMIN capability
- Automatic fallback to standard mode if capabilities absent
- Zero-copy packet processing via AF_XDP (future implementation)

To enable XDP:
1. Build with `-tags xdp`
2. Run with `DBPROXY_XDP_ENABLED=true`
3. Ensure NET_ADMIN capability (or SYS_ADMIN on older kernels)

## Security Model

### SQL Injection Detection
- Regex-based pattern matching on query strings
- Heuristic: flags queries with >3 different SQL keywords
- Detects: union-based, comment-based, stacked queries, time-based blind
- **Note**: Not production-safe on its own; should be combined with parameterized queries in applications

### Blocked Resources
- Configurable list of keywords/patterns to reject
- Examples: `DROP TABLE`, `TRUNCATE`, `DELETE FROM`

### Tenant Isolation
- Routes are tagged with `tenant` ID
- Proxy enforces routing to tenant-specific backends
- Auth middleware should validate tenant claim before connecting

### Capabilities
- Runs as UID 1000 (unprivileged)
- Requires no system-level permissions for standard mode
- Requires NET_ADMIN + NET_RAW for XDP mode (or CAP_BPF on Linux 5.8+)

## Metrics

### Prometheus

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `nest_dbproxy_active_connections` | Gauge | `protocol` | Active connections |
| `nest_dbproxy_total_connections` | Counter | `protocol` | Total connections established |
| `nest_dbproxy_queries_blocked_total` | Counter | `protocol` | Queries blocked by security checker |
| `nest_dbproxy_network_mode` | Gauge | | Active network mode (0=standard, 1=xdp) |

## Testing

```bash
go test -v -race -cover ./...
go test -bench=. -benchmem ./...
```

Coverage: 90%+ (required)

## Development

### Project structure
```
services/db-proxy/
├── cmd/
│   └── main.go              # Entry point
├── internal/
│   ├── handlers/            # TCP proxy handlers
│   ├── pool/                # Connection pooling
│   ├── security/            # SQL injection checker
│   ├── config/              # Config loading from Redis
│   ├── net_standard.go      # Standard networking (noxdp)
│   └── net_xdp.go           # XDP networking (xdp tag)
├── proto/
│   └── config.proto         # gRPC config API
├── tests/                   # Integration tests
├── Dockerfile
├── go.mod / go.sum
└── CONFIG_CONTRACT.md       # Config schema & gRPC API
```

### Adding a new protocol
1. Create `internal/handlers/{protocol}.go` with protocol-specific parsing (optional)
2. Register in `cmd/main.go` with new port
3. Update tests

### Extending security checks
1. Add regex pattern to `security.Checker.patterns` (or via `AddPattern`)
2. Update `CONFIG_CONTRACT.md` with new check documentation
3. Add tests to `internal/security/checker_test.go`

## Limitations

- Regex-based SQL injection detection is heuristic-only; use parameterized queries in applications
- Connection pooling is per-proxy instance (not cluster-aware)
- No query-level authentication/authorization; use app-level access controls
- No compression or encryption at proxy level; use TLS at application level

## Future Work

1. Full AF_XDP zero-copy implementation
2. Query-level caching via Redis
3. Cluster mode (multiple proxy instances with shared state)
4. Protocol-specific parsing (MySQL COM_QUERY handler, PostgreSQL wire protocol)
5. Metrics-based autoscaling
6. gRPC streaming for high-volume metrics
