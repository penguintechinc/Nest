# NEST API Documentation

NEST (Network-Enabled Security & Topology) platform API reference for the Manager service.

## Base URL

| Environment | Base URL |
|-------------|----------|
| Alpha (local) | `https://nest.localhost.local/api/v1` |
| Beta | `https://nest.penguintech.cloud/api/v1` |
| Production | `https://nest.penguincloud.io/api/v1` |

## Authentication

All endpoints (unless noted) require a JWT Bearer token in the `Authorization` header:

```
Authorization: Bearer <token>
```

Obtain a token via `POST /api/v1/auth/login`. Tokens use HS256 signing and are configured via the `JWT_SECRET` environment variable.

### Roles

| Role | Description |
|------|-------------|
| `admin` | Full system access, user management |
| `maintainer` | Read/write access, no user management |
| `viewer` | Read-only access |

---

## 1. Auth (`/api/v1/auth/`)

### POST `/api/v1/auth/login`

Login and receive a JWT token. **No auth required.**

**Request:**
```json
{ "username": "admin", "password": "secret" }
```
`username` may be a username or email address.

**Response (200):**
```json
{
  "token": "eyJ...",
  "user": { "id": 1, "username": "admin", "email": "admin@example.com", "role": "admin", "is_active": true }
}
```

### POST `/api/v1/auth/logout`

Stateless logout. Auth required.

**Response (200):** `{ "message": "successfully logged out" }`

### GET `/api/v1/auth/me`

Get the current authenticated user's profile. Auth required.

**Response (200):** `{ "user": { ... } }`

### POST `/api/v1/auth/register`

Create a new user account. **Admin only.**

**Request:**
```json
{ "username": "jdoe", "email": "jdoe@example.com", "password": "pass", "first_name": "Jane", "last_name": "Doe", "role": "viewer" }
```
`role` must be one of: `admin`, `maintainer`, `viewer`.

**Response (201):** User object. **409** if username/email already exists.

---

## 2. Teams (`/api/v1/teams/`)

### GET `/api/v1/teams`

List teams. Non-admins see only their teams. Auth required.

**Response (200):** `{ "teams": [...], "count": N }`

### POST `/api/v1/teams`

Create a team. **Admin only.**

**Request:** `{ "name": "DevOps", "description": "..." }`

**Response (201):** Team object. **409** on duplicate name.

### GET `/api/v1/teams/<team_id>`

Get a team with members. Auth required (must be member or admin).

**Response (200):** Team object with `members` array.

### PUT `/api/v1/teams/<team_id>`

Update team. **Maintainer+ role** (must be team_admin or global admin).

**Request:** `{ "name": "NewName", "description": "..." }`

**Response (200):** Updated team object.

### DELETE `/api/v1/teams/<team_id>`

Soft-delete a team. **Admin only.** Global teams cannot be deleted.

**Response (200):** `{ "message": "Team deleted successfully" }`

### GET `/api/v1/teams/<team_id>/members`

List team members. Auth required (must be member or admin).

**Response (200):** `{ "members": [...], "count": N }`

### POST `/api/v1/teams/<team_id>/members`

Add a member to a team. **Admin only.**

**Request:** `{ "user_id": 2, "role": "team_admin" }`

`role` must be one of: `team_admin`, `team_maintainer`, `team_viewer`.

**Response (201):** Membership object. **409** if already a member.

### DELETE `/api/v1/teams/<team_id>/members/<user_id>`

Remove a member from a team. **Admin only.**

**Response (200):** `{ "message": "Team member removed successfully" }`

---

## 3. Database Servers (`/api/v1/servers/`)

### GET `/api/v1/servers`

List active database servers (paginated). **No auth required.**

**Query params:** `page` (default 1), `per_page` (default 20, max 100).

**Response (200):** `{ "data": [...], "meta": { "page": 1, "per_page": 20, "total": N } }`

### POST `/api/v1/servers`

Create a database server entry. Auth required.

**Request:**
```json
{ "name": "prod-db", "host": "db.example.com", "port": 5432, "db_type": "postgresql", "username": "admin", "region": "us-east-1", "tags": "prod" }
```
Required: `name`, `host`, `port`, `db_type`.

**Response (201):** `{ "data": { ... } }`

### GET `/api/v1/servers/<server_id>`

Get a single server. Auth required.

### PUT `/api/v1/servers/<server_id>`

Update a server. Auth required. Updatable fields: `name`, `host`, `port`, `db_type`, `username`, `region`, `tags`.

### DELETE `/api/v1/servers/<server_id>`

Soft-delete (deactivate) a server. **Admin only.**

**Response (200):** `{ "message": "Server deactivated" }`

### POST `/api/v1/servers/<server_id>/test`

Test TCP connectivity to a database server. Auth required.

**Response (200):**
```json
{ "server_id": 1, "host": "db.example.com", "port": 5432, "reachable": true, "error": null }
```

---

## 4. Managed Databases (`/api/v1/databases/`)

### GET `/api/v1/databases`

List all managed database entries. Auth required.

### POST `/api/v1/databases`

Create a managed database entry. Auth required.

**Request:** `{ "server_id": 1, "db_name": "mydb", "description": "...", "db_type": "postgresql", "size_bytes": 0 }`

Required: `server_id`, `db_name`.

### GET `/api/v1/databases/<db_id>`

Get a managed database entry. Auth required.

### PUT `/api/v1/databases/<db_id>`

Update a managed database. Auth required. Updatable: `db_name`, `description`, `db_type`, `size_bytes`.

### DELETE `/api/v1/databases/<db_id>`

Delete a managed database entry. **Admin only.**

### GET `/api/v1/databases/<db_id>/schema`

Get schema information (tables/columns) for a managed database. Auth required.

**Response (200):** `{ "data": { "database": { ... }, "schema": [ ... ] } }`

### POST `/api/v1/databases/<db_id>/schema`

Trigger a schema refresh job (async). Auth required.

**Response (202):** `{ "message": "Schema refresh queued", "db_id": 1 }`

---

## 5. SQL Files (`/api/v1/sql-files/`)

### GET `/api/v1/sql-files`

List all SQL files (metadata only, no content). Auth required.

### POST `/api/v1/sql-files`

Upload a SQL file. Auth required.

**Request:** `{ "filename": "migration.sql", "content": "CREATE TABLE ..." }`

### GET `/api/v1/sql-files/<file_id>`

Get SQL file metadata and content. Auth required.

### DELETE `/api/v1/sql-files/<file_id>`

Delete a SQL file. **Admin only.**

### POST `/api/v1/sql-files/<file_id>/validate`

Validate SQL file security (runs in subprocess pool). Auth required.

**Response (200):** `{ "file_id": 1, "valid": true, "result": { ... } }`

### POST `/api/v1/sql-files/<file_id>/execute`

Execute a SQL file. **Admin only.** Currently returns **501 Not Implemented**.

---

## 6. Security Rules (`/api/v1/security-rules/`)

### GET `/api/v1/security-rules`

List all security rules, sorted by priority. Auth required.

### POST `/api/v1/security-rules`

Create a security rule. **Admin only.**

**Request:**
```json
{ "name": "Block DROP", "rule_type": "sql_pattern", "action": "block", "priority": 1, "pattern": "DROP TABLE", "description": "...", "enabled": true }
```
Required: `name`, `rule_type`, `action`, `priority`.

### GET `/api/v1/security-rules/<rule_id>`

Get a single security rule. Auth required.

### PUT `/api/v1/security-rules/<rule_id>`

Update a security rule. **Admin only.** Updatable: `name`, `rule_type`, `action`, `priority`, `pattern`, `description`, `enabled`.

### DELETE `/api/v1/security-rules/<rule_id>`

Delete a security rule. **Admin only.**

---

## 7. Threat Intel (`/api/v1/threat-intel/`)

### Feeds

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/threat-intel/feeds` | Auth | List all feeds |
| POST | `/threat-intel/feeds` | Admin | Create a feed. Required: `name`, `url`, `feed_type`. Optional: `poll_interval_minutes`, `enabled`. |
| GET | `/threat-intel/feeds/<feed_id>` | Auth | Get a feed |
| PUT | `/threat-intel/feeds/<feed_id>` | Admin | Update a feed. Updatable: `name`, `url`, `feed_type`, `poll_interval_minutes`, `enabled`. |
| DELETE | `/threat-intel/feeds/<feed_id>` | Admin | Delete a feed |
| POST | `/threat-intel/feeds/<feed_id>/poll` | Admin | Trigger immediate poll (202) |

### Indicators

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/threat-intel/indicators` | Auth | List indicators. Filters: `type`, `feed_id`. Max 500 results. |

### Matches

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/threat-intel/matches` | Auth | List threat matches. Filter: `server_id`. Max 500 results. |

---

## 8. Blocked Databases (`/api/v1/blocked-databases/`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/blocked-databases` | Auth | List all blocked databases |
| POST | `/blocked-databases` | Admin | Add a blocked database. Required: `db_name`. Optional: `server_id`, `reason`. |
| GET | `/blocked-databases/<block_id>` | Auth | Get a block entry |
| DELETE | `/blocked-databases/<block_id>` | Admin | Remove a block entry |

---

## 9. Cloud Providers (`/api/v1/cloud/`)

### Providers

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/cloud/providers` | Auth | List providers (credentials omitted) |
| POST | `/cloud/providers` | Admin | Create provider. Required: `name`, `provider_type`. Optional: `region`, `credentials` (encrypted at rest). |
| GET | `/cloud/providers/<prov_id>` | Auth | Get provider (credentials omitted) |
| PUT | `/cloud/providers/<prov_id>` | Admin | Update provider. Updatable: `name`, `provider_type`, `region`, `credentials`. |
| DELETE | `/cloud/providers/<prov_id>` | Admin | Delete provider |

### Instances

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/cloud/instances` | Auth | List all cloud instances |
| POST | `/cloud/instances` | Admin | Create instance. Required: `provider_id`, `instance_id`, `instance_type`. Optional: `region`, `status`, `ip_address`. |
| PUT | `/cloud/instances/<inst_id>` | Auth | Update instance. Updatable: `status`, `ip_address`, `instance_type`, `region`. |

---

## 10. Scaling Policies (`/api/v1/scaling/`)

### Policies

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/scaling/policies` | Auth | List all scaling policies |
| POST | `/scaling/policies` | Admin | Create policy. Required: `name`, `server_id`, `metric`, `threshold`. Optional: `scale_up_by`, `scale_down_by`, `cooldown_seconds`, `enabled`. |
| GET | `/scaling/policies/<pol_id>` | Auth | Get a policy |
| PUT | `/scaling/policies/<pol_id>` | Admin | Update policy. Updatable: `name`, `metric`, `threshold`, `scale_up_by`, `scale_down_by`, `cooldown_seconds`, `enabled`. |
| DELETE | `/scaling/policies/<pol_id>` | Admin | Delete a policy |

### Events

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/scaling/events` | Auth | List scaling events. Filters: `server_id`, `status`. Max 200 results. |

---

## 11. Temporary Access (`/api/v1/temporary-access/`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/temporary-access` | Admin | List all temporary access tokens |
| POST | `/temporary-access` | Admin | Create a token. Required: `user_id`, `server_id`. Optional: `ttl_hours` (1-720, default 24). |
| GET | `/temporary-access/<token_id>` | Auth | Get token details |
| DELETE | `/temporary-access/<token_id>` | Admin | Delete a token |
| POST | `/temporary-access/<token_id>/revoke` | Admin | Revoke a token (sets `used_at` to now) |

---

## 12. Permissions (`/api/v1/permissions/`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/permissions` | Auth | List permissions. Filters: `user_id`, `server_id`. |
| POST | `/permissions` | Admin | Create permission. Required: `user_id`, `server_id`, `permission_level`. |
| GET | `/permissions/<perm_id>` | Auth | Get a permission |
| DELETE | `/permissions/<perm_id>` | Admin | Delete a permission |
| GET | `/users/<user_id>/permissions` | Auth | Get all permissions for a user |

---

## 13. User Profiles (`/api/v1/users/`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/users/<user_id>/profile` | Auth | Get user profile (API key omitted) |
| PUT | `/users/<user_id>/profile` | Auth | Update profile. Updatable: `rate_limit`, `ip_whitelist`, `display_name`, `timezone`. Non-admins can only update their own. |
| POST | `/users/<user_id>/regenerate-api-key` | Auth | Regenerate API key. Returns plaintext key once. Non-admins can only regenerate their own. |

---

## 14. Sync (`/api/v1/sync/`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/sync` | Auth | Sync database_server rows to Redis and trigger DB Proxy reload. Returns **207** if Redis sync succeeds but DB Proxy reload fails. |
| GET | `/blocking-config` | Auth | Get current blocking config from Redis/DB Proxy |
| PUT | `/blocking-config` | Admin | Update blocking config in Redis/DB Proxy |
| POST | `/seed-blocked-resources` | Admin | Seed default blocked resources (information_schema, mysql, sys, etc.) |

---

## 15. Audit (`/api/v1/audit-log/`)

### GET `/api/v1/audit-log`

Paginated audit log with optional filters. Auth required.

**Query params:** `page` (default 1), `per_page` (default 50, max 200), `user_id`, `server_id`, `action`, `from` (timestamp), `to` (timestamp).

**Response (200):** `{ "data": [...], "meta": { "page": 1, "per_page": 50, "total": N } }`

---

## 16. Stats (`/api/v1/stats/`)

### GET `/api/v1/status`

Service status endpoint. **No auth required.**

**Response (200):**
```json
{ "version": "1.1.0", "build_epoch": 0, "service": "nest-manager", "status": "ok" }
```

### GET `/api/v1/stats`

Aggregate statistics. Auth required.

**Response (200):**
```json
{
  "data": {
    "servers": { "total": 10, "active": 8 },
    "managed_databases": 25,
    "permissions": 15,
    "threat_indicators": 1200,
    "threat_feeds": 3,
    "security_rules": 12,
    "blocked_databases": 6
  }
}
```

---

## 17. License (`/api/v1/license/`)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/license` | Auth | Get current license info (key masked). Returns `null` data if no license configured. |
| POST | `/license` | Admin | Validate and store a license key. Request: `{ "license_key": "PENG-..." }`. Returns **422** if validation fails. |
| DELETE | `/license` | Admin | Remove license information |

---

## 18. Analytics (`/api/v1/`)

### GET `/api/v1/advanced/analytics`

Aggregate analytics data. Auth required (any role).

**Response (200):**
```json
{
  "message": "Advanced analytics data",
  "data": {
    "resources": { "total": 50, "active": 40, "by_status": { "running": 30, "stopped": 20 } },
    "audit_log": { "events_last_7_days": 150 },
    "teams": { "total": 5 },
    "users": { "active": 12 }
  }
}
```

### GET `/api/v1/enterprise/reports`

Enterprise report data. **Admin only** (license-gated feature).

**Response (200):**
```json
{
  "message": "Enterprise reports",
  "reports": ["security_audit", "compliance_report", "usage_analytics"],
  "data": {
    "security_audit": { "blocked_databases": 6, "active_security_rules": 12, "certificates_expiring_in_30_days": 2 },
    "compliance_report": { "backup_jobs_completed_30d": 28, "backup_jobs_failed_30d": 1, "provisioning_jobs_30d": 5 },
    "usage_analytics": { "total_resources": 50, "total_teams": 5, "total_active_users": 12 }
  }
}
```

---

## Error Responses

All errors follow this format:

```json
{ "error": "error_code_or_message" }
```

| Status | Meaning |
|--------|---------|
| 400 | Bad request / validation error |
| 401 | Invalid credentials or missing/expired token |
| 403 | Insufficient permissions |
| 404 | Resource not found |
| 409 | Conflict (duplicate resource) |
| 422 | Unprocessable entity (e.g., license validation failed) |
| 500 | Internal server error |
| 501 | Not implemented |
