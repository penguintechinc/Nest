# DB Proxy Configuration Contract

This document defines the configuration interface between the DB Proxy and the Manager service (or any external config source). The DB Proxy consumes configuration via two channels:

1. **Redis key-value store** — for dynamic, watchable configuration
2. **gRPC ConfigService API** — for querying and reloading configuration

## Redis Configuration Schema

All configuration keys are prefixed with `DBPROXY_REDIS_PREFIX` (default: `nest:dbproxy`).

### Routes Configuration
**Key:** `{prefix}:routes`  
**Type:** JSON  
**Schema (V2 — Read/Write Routing):**
```json
{
  "routes": {
    "{route_id}": {
      "protocol": "mysql|postgresql|redis|mongodb|mssql",
      "tenant": "tenant-uuid",
      "primary": {
        "name": "primary",
        "backend": "hostname or IP",
        "port": 3306,
        "max_connections": 20
      },
      "replicas": [
        {
          "name": "replica-1",
          "backend": "replica-1.internal",
          "port": 3306,
          "max_connections": 10
        },
        {
          "name": "replica-2",
          "backend": "replica-2.internal",
          "port": 3306,
          "max_connections": 10
        }
      ]
    }
  }
}
```

**Example:**
```json
{
  "routes": {
    "tenant-1-main": {
      "protocol": "mysql",
      "tenant": "tenant-1",
      "primary": {
        "name": "primary",
        "backend": "tenant-1-db-primary.internal",
        "port": 3306,
        "max_connections": 20
      },
      "replicas": [
        {
          "name": "replica-1",
          "backend": "tenant-1-db-replica-1.internal",
          "port": 3306,
          "max_connections": 10
        },
        {
          "name": "replica-2",
          "backend": "tenant-1-db-replica-2.internal",
          "port": 3306,
          "max_connections": 10
        }
      ]
    }
  }
}
```

**Routing Behavior:**
- **SELECT, SHOW, EXPLAIN queries** → routed to a healthy replica (if available); falls back to primary
- **INSERT, UPDATE, DELETE, DDL, CALL queries** → routed to primary
- **BEGIN, COMMIT, ROLLBACK** → routed to primary; subsequent queries in the transaction stay on primary (consistency)
- **PostgreSQL extended protocol (Parse)** → routed to primary (deferred full support)
- **Unknown query types** → routed to primary (fail-safe)
- **Replica health check** → TCP dial every 5 seconds; unmarked replicas automatically restored

### Security Configuration
**Key:** `{prefix}:security`  
**Type:** JSON  
**Schema:**
```json
{
  "blocked_resources": ["DROP TABLE", "TRUNCATE", "DELETE FROM"],
  "allowed_resources": [],
  "enable_injection_check": true
}
```

**Behavior:**
- If `enable_injection_check: true`, the proxy uses regex patterns to detect SQL injection attempts
- Blocked resources are keywords or patterns that trigger immediate rejection
- Allowed resources (whitelist) can bypass some checks if needed

### Blocking Configuration (Intent #3 — Blue/Green + Multi-Write)
**Key:** `{prefix}:routes`  
**Type:** JSON (extended schema below)  
**Purpose:** Blue/green primary switching and multi-write targeting for HA

**Schema (V3 — with Blue/Green and Multi-Write):**

The `routes` key now supports optional `blue_green` and `multi_write` fields:

```json
{
  "routes": {
    "{route_id}": {
      "protocol": "mysql|postgresql",
      "tenant": "tenant-uuid",
      "primary": {
        "name": "primary",
        "backend": "hostname",
        "port": 3306,
        "max_connections": 20
      },
      "blue_green": {
        "blue": {
          "name": "blue",
          "backend": "blue-primary.internal",
          "port": 3306,
          "max_connections": 20
        },
        "green": {
          "name": "green",
          "backend": "green-primary.internal",
          "port": 3306,
          "max_connections": 20
        },
        "active": "blue"
      },
      "multi_write": {
        "enabled": true,
        "write_targets": [
          {
            "name": "primary-1",
            "backend": "primary-1.internal",
            "port": 3306,
            "max_connections": 20,
            "authoritative": true
          },
          {
            "name": "primary-2",
            "backend": "primary-2.internal",
            "port": 3306,
            "max_connections": 20,
            "authoritative": false
          }
        ],
        "consistency_policy": "best-effort"
      },
      "replicas": [...]
    }
  }
}
```

**Blue/Green Switching:**
- Allows two candidate primary endpoint sets: "blue" and "green"
- One is marked `"active"`: either `"blue"` or `"green"`
- Writes (and reads pinned to primary) go to the ACTIVE primary
- Config update flips `"active"` field, enabling zero-downtime cutover
- Backward compatible: if `blue_green` is absent, uses legacy single `primary` field

**Multi-Write Configuration:**
- `"enabled"`: bool (default false)
- `"write_targets"`: array of write target endpoints
- `"consistency_policy"`: either `"best-effort"` (default) or `"strict"`
- Each write target has `"authoritative": true/false`
- On a WRITE: send same request to ALL targets, relay response from authoritative target
- **best-effort mode:** secondary write failures log + count metric but don't fail client
- **strict mode:** any secondary failure fails the client write

**Backward Compatibility:**
- Routes with no `blue_green` field use legacy single `primary`
- Routes with no `multi_write` field use single write target (primary)
- Existing routes continue to work unchanged

### Cache Configuration (Intent #4 — Future)
**Key:** `{prefix}:cache`  
**Type:** JSON (reserved for future use)  
**Purpose:** Query result caching for read-path optimization  
**Structure:** To be defined when intent #4 is implemented.  
**Note:** The router will expose a cache-lookup hook on the read path before forwarding to replica.

## Configuration Watching

The proxy watches for configuration changes in Redis via:
- **Pub/Sub channel:** `{prefix}:config:updated` — emitted by Manager after writing config
- **Key watchers:** Periodic polling on configuration keys (fallback if Pub/Sub unavailable)

When a configuration update is detected:
1. The proxy reloads the affected configuration section
2. New connections use the updated configuration
3. Existing connections complete with old configuration
4. Metrics track reload events

## gRPC ConfigService API

Proto definition: `proto/config.proto`  
Port: `DBPROXY_GRPC_PORT` (default: 50051)

All requests must include `api_version: "v1"` field.

### GetConfig
Retrieve current configuration.

**Request:**
```protobuf
message GetConfigRequest {
  string api_version = 1;  // "v1"
  string config_key = 2;   // "routes", "security", "blocking", "cache" (optional)
}
```

**Response:**
```protobuf
message GetConfigResponse {
  string api_version = 1;
  string status = 2;       // "success" or "error"
  string error_message = 3;
  bytes config_data = 4;   // JSON-encoded config
}
```

**Example:**
```bash
grpcurl -plaintext \
  -d '{"api_version": "v1", "config_key": "routes"}' \
  localhost:50051 \
  nest.dbproxy.v1.ConfigService/GetConfig
```

### SetConfig
Update configuration (called by Manager).

**Request:**
```protobuf
message SetConfigRequest {
  string api_version = 1;  // "v1"
  string config_key = 2;   // "routes", "security", "blocking", or "cache"
  bytes config_data = 3;   // JSON-encoded config
}
```

**Response:**
```protobuf
message SetConfigResponse {
  string api_version = 1;
  string status = 2;
  string error_message = 3;
}
```

### ReloadConfig
Trigger a configuration reload from Redis.

**Request:**
```protobuf
message ReloadConfigRequest {
  string api_version = 1;  // "v1"
}
```

**Response:**
```protobuf
message ReloadConfigResponse {
  string api_version = 1;
  string status = 2;
  string error_message = 3;
  string reload_timestamp = 4;
}
```

### GetStatus
Retrieve proxy runtime status.

**Request:**
```protobuf
message GetStatusRequest {
  string api_version = 1;  // "v1"
}
```

**Response:**
```protobuf
message GetStatusResponse {
  string api_version = 1;
  string status = 2;
  int64 active_connections = 3;
  int64 total_connections = 4;
  int64 queries_processed = 5;
  int64 queries_blocked = 6;
  string uptime_seconds = 7;
}
```

## Configuration Manager Responsibilities

When the Manager updates routes or security policy, it MUST:

1. **Write to Redis** using the schema above:
   ```bash
   redis-cli SET nest:dbproxy:routes '{"routes":{...}}'
   redis-cli PUBLISH nest:dbproxy:config:updated "routes"
   ```

2. **Or call gRPC SetConfig:**
   ```bash
   grpcurl -d '{"api_version":"v1","config_key":"routes","config_data":"eyJyb3V0ZXMiOnsuLi59fQ=="}' \
     localhost:50051 nest.dbproxy.v1.ConfigService/SetConfig
   ```

3. **Ensure Redis connectivity:** The proxy will not start if Redis is unreachable (no graceful degradation)

## Environment Variables

- `DBPROXY_REDIS_ADDR` — Redis host (default: localhost)
- `DBPROXY_REDIS_PORT` — Redis port (default: 6379)
- `DBPROXY_REDIS_DB` — Redis DB index (default: 0)
- `DBPROXY_REDIS_PREFIX` — Configuration key prefix (default: `nest:dbproxy`)
- `DBPROXY_XDP_ENABLED` — Enable XDP/AF_XDP fast path (default: false; requires NET_ADMIN)

## Tenant Isolation

Configuration is tenant-scoped via the `tenant` field in routes. The proxy does NOT enforce tenant isolation at the protocol level; that is the responsibility of the upstream auth middleware and the backend database.

Example:
```json
{
  "routes": {
    "tenant-1-primary": {
      "protocol": "mysql",
      "backend": "tenant-1.db.internal",
      "port": 3306,
      "tenant": "tenant-1"
    },
    "tenant-2-primary": {
      "protocol": "mysql",
      "backend": "tenant-2.db.internal",
      "port": 3306,
      "tenant": "tenant-2"
    }
  }
}
```

The proxy forwards all traffic to the configured backend; tenant validation happens at the API gateway or auth middleware level.
