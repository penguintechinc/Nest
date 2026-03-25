# 🔌 ArticDBM API Reference

This document provides comprehensive documentation for all ArticDBM API endpoints, including the new database management and security features.

## 📖 Table of Contents

- [Authentication](#authentication)
- [Server Management](#server-management)
- [User & Permission Management](#user--permission-management)
- [Database Management](#database-management)
- [Cloud Provider Management](#cloud-provider-management) ⭐ NEW in v1.1.0
- [Cloud Database Instances](#cloud-database-instances) ⭐ NEW in v1.1.0
- [Auto-Scaling Policies](#auto-scaling-policies) ⭐ NEW in v1.1.0
- [SQL File Management](#sql-file-management)
- [Security & Blocking](#security--blocking)
- [Monitoring & Statistics](#monitoring--statistics)
- [Error Handling](#error-handling)

## 🔐 Authentication

All API endpoints require authentication using session-based authentication or API tokens.

### Login
```http
POST /auth/login
Content-Type: application/json

{
  "email": "admin@example.com",
  "password": "secure_password"
}
```

**Response:**
```json
{
  "success": true,
  "user": {
    "id": 1,
    "email": "admin@example.com",
    "created_on": "2024-01-01T00:00:00Z"
  }
}
```

## 🖥️ Server Management

### List Database Servers
```http
GET /api/servers
Authorization: Bearer <token>
```

**Response:**
```json
{
  "servers": [
    {
      "id": 1,
      "name": "mysql-primary",
      "type": "mysql",
      "host": "192.168.1.10",
      "port": 3306,
      "role": "write",
      "weight": 1,
      "tls_enabled": false,
      "active": true
    }
  ]
}
```

### Add Database Server
```http
POST /api/servers
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "mysql-primary",
  "type": "mysql",
  "host": "192.168.1.10",
  "port": 3306,
  "username": "root",
  "password": "secret",
  "database": "myapp",
  "role": "write",
  "weight": 1,
  "tls_enabled": false,
  "active": true
}
```

**Response:**
```json
{
  "id": 1,
  "message": "Server created successfully"
}
```

### Update Database Server
```http
PUT /api/servers/{server_id}
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "mysql-primary-updated",
  "weight": 2,
  "active": true
}
```

### Delete Database Server
```http
DELETE /api/servers/{server_id}
Authorization: Bearer <token>
```

**Response:**
```json
{
  "message": "Server deleted successfully"
}
```

## 👥 User & Permission Management

### List Permissions
```http
GET /api/permissions
Authorization: Bearer <token>
```

**Response:**
```json
{
  "permissions": [
    {
      "id": 1,
      "user_id": 1,
      "username": "user@example.com",
      "database_name": "production_db",
      "table_name": "users",
      "actions": ["read", "write"]
    }
  ]
}
```

### Create Permission
```http
POST /api/permissions
Content-Type: application/json
Authorization: Bearer <token>

{
  "user_id": 1,
  "database_name": "production_db",
  "table_name": "users",
  "actions": ["read"]
}
```

### Update Permission
```http
PUT /api/permissions/{perm_id}
Content-Type: application/json
Authorization: Bearer <token>

{
  "actions": ["read", "write"]
}
```

### Delete Permission
```http
DELETE /api/permissions/{perm_id}
Authorization: Bearer <token>
```

## 🗃️ Database Management

### List Managed Databases
```http
GET /api/databases
Authorization: Bearer <token>
```

**Response:**
```json
{
  "databases": [
    {
      "id": 1,
      "name": "production_app",
      "database_name": "prod_db",
      "server_name": "mysql-primary",
      "server_type": "mysql",
      "description": "Main production database",
      "schema_version": "v20240101_120000",
      "auto_backup": true,
      "backup_schedule": "0 2 * * *",
      "active": true,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Create Managed Database
```http
POST /api/databases
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "production_app",
  "server_id": 1,
  "database_name": "prod_db",
  "description": "Main production database",
  "schema_version": "v1.0.0",
  "auto_backup": true,
  "backup_schedule": "0 2 * * *",
  "active": true
}
```

**Response:**
```json
{
  "id": 1,
  "message": "Managed database created successfully"
}
```

### Update Managed Database
```http
PUT /api/databases/{database_id}
Content-Type: application/json
Authorization: Bearer <token>

{
  "description": "Updated production database",
  "auto_backup": false
}
```

### Delete Managed Database
```http
DELETE /api/databases/{database_id}
Authorization: Bearer <token>
```

### Get Database Schema
```http
GET /api/databases/{database_id}/schema
Authorization: Bearer <token>
```

**Response:**
```json
{
  "database_name": "production_app",
  "tables": [
    {
      "name": "users",
      "columns": [
        {
          "name": "id",
          "data_type": "INTEGER",
          "is_nullable": false,
          "is_primary_key": true,
          "is_foreign_key": false
        },
        {
          "name": "email",
          "data_type": "VARCHAR(255)",
          "is_nullable": false,
          "is_primary_key": false,
          "is_foreign_key": false
        }
      ]
    }
  ]
}
```

### Update Database Schema
```http
POST /api/databases/{database_id}/schema
Content-Type: application/json
Authorization: Bearer <token>

{
  "schema": [
    {
      "name": "users",
      "columns": [
        {
          "name": "id",
          "data_type": "INTEGER",
          "is_nullable": false,
          "is_primary_key": true
        },
        {
          "name": "email",
          "data_type": "VARCHAR(255)",
          "is_nullable": false
        }
      ]
    }
  ]
}
```

## ☁️ Cloud Provider Management

### List Cloud Providers
```http
GET /api/cloud-providers
Authorization: Bearer <token>
```

**Response:**
```json
{
  "providers": [
    {
      "id": 1,
      "name": "production-k8s",
      "provider_type": "kubernetes",
      "is_active": true,
      "test_status": "success",
      "created_at": "2025-01-15T10:00:00Z",
      "last_tested": "2025-01-15T10:05:00Z"
    }
  ]
}
```

### Create Cloud Provider
```http
POST /api/cloud-providers
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "production-aws",
  "provider_type": "aws",
  "configuration": {
    "region": "us-east-1",
    "vpc_id": "vpc-12345",
    "subnet_group_name": "articdbm-subnet-group",
    "security_group_ids": ["sg-12345", "sg-67890"]
  },
  "credentials_path": "/secure/aws-credentials.json",
  "is_active": true
}
```

**Supported Provider Types:**
- `kubernetes` - Kubernetes cluster integration
- `aws` - Amazon Web Services (RDS, ElastiCache)
- `gcp` - Google Cloud Platform (Cloud SQL, Spanner)

### Test Cloud Provider Connection
```http
POST /api/cloud-providers/1/test
Authorization: Bearer <token>
```

**Response:**
```json
{
  "test_result": "success"
}
```

## 🖥️ Cloud Database Instances

### List Cloud Instances
```http
GET /api/cloud-instances
Authorization: Bearer <token>
```

**Response:**
```json
{
  "instances": [
    {
      "id": 1,
      "name": "production-mysql",
      "provider_name": "production-aws",
      "instance_type": "mysql",
      "instance_class": "db.t3.medium",
      "status": "available",
      "endpoint": "articdbm-production-mysql.c1234.us-east-1.rds.amazonaws.com",
      "port": 3306,
      "created_at": "2025-01-15T10:15:00Z"
    }
  ]
}
```

### Create Cloud Database Instance
```http
POST /api/cloud-instances
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "app-postgres",
  "provider_id": 1,
  "instance_type": "postgresql",
  "instance_class": "db.t3.medium",
  "storage_size": 100,
  "engine_version": "15.4",
  "multi_az": true,
  "backup_retention": 7,
  "monitoring_enabled": true,
  "auto_scaling_enabled": true,
  "auto_scaling_config": {
    "min_capacity": 1,
    "max_capacity": 10,
    "target_cpu": 70
  }
}
```

**Instance Types:**
- `mysql` - MySQL database
- `postgresql` - PostgreSQL database
- `mssql` - Microsoft SQL Server
- `mongodb` - MongoDB (where supported)
- `redis` - Redis cache

### Scale Cloud Instance
```http
POST /api/cloud-instances/1/scale
Authorization: Bearer <token>
Content-Type: application/json

{
  "action": "scale_up",
  "instance_class": "db.t3.large",
  "ai_enabled": true
}
```

**Actions:**
- `scale_up` - Increase instance capacity
- `scale_down` - Decrease instance capacity

## 📊 Auto-Scaling Policies

### List Scaling Policies
```http
GET /api/scaling-policies
Authorization: Bearer <token>
```

**Response:**
```json
{
  "policies": [
    {
      "id": 1,
      "instance_name": "production-mysql",
      "metric_type": "cpu",
      "scale_up_threshold": 80.0,
      "scale_down_threshold": 20.0,
      "ai_enabled": true,
      "ai_model": "openai",
      "is_active": true
    }
  ]
}
```

### Create Scaling Policy
```http
POST /api/scaling-policies
Authorization: Bearer <token>
Content-Type: application/json

{
  "cloud_instance_id": 1,
  "metric_type": "cpu",
  "scale_up_threshold": 80.0,
  "scale_down_threshold": 20.0,
  "scale_up_adjustment": 1,
  "scale_down_adjustment": -1,
  "cooldown_period": 300,
  "ai_enabled": true,
  "ai_model": "openai",
  "is_active": true
}
```

**Metric Types:**
- `cpu` - CPU utilization percentage
- `memory` - Memory utilization percentage
- `connections` - Active database connections
- `iops` - Input/output operations per second

**AI Models:**
- `openai` - OpenAI GPT-4 for scaling recommendations
- `anthropic` - Anthropic Claude for optimization
- `ollama` - Local Ollama for on-premise AI

## 📄 SQL File Management

### List SQL Files
```http
GET /api/sql-files?database_id=1
Authorization: Bearer <token>
```

**Response:**
```json
{
  "sql_files": [
    {
      "id": 1,
      "name": "user_table_migration.sql",
      "database_name": "production_app",
      "file_type": "migration",
      "file_size": 245,
      "syntax_validated": true,
      "security_validated": true,
      "validation_errors": null,
      "executed": false,
      "executed_at": null,
      "executed_by": null,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Upload SQL File
```http
POST /api/sql-files
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "user_table_migration.sql",
  "database_id": 1,
  "file_type": "migration",
  "file_content": "CREATE TABLE users (id INT PRIMARY KEY, email VARCHAR(255));"
}
```

**Response:**
```json
{
  "id": 1,
  "message": "SQL file uploaded successfully",
  "validation": {
    "valid": true,
    "errors": [],
    "warnings": [],
    "severity": "low",
    "patterns_checked": 40
  }
}
```

### Execute SQL File
```http
POST /api/sql-files/{file_id}/execute
Authorization: Bearer <token>
```

**Response:**
```json
{
  "message": "SQL file user_table_migration.sql executed successfully"
}
```

### Validate SQL File
```http
POST /api/sql-files/{file_id}/validate
Authorization: Bearer <token>
```

**Response:**
```json
{
  "message": "SQL file validated successfully",
  "validation": {
    "valid": false,
    "errors": [
      "Shell command detected - Command shell reference: \\bcmd\\b"
    ],
    "warnings": [
      "Potential default/test resource access - Test database access"
    ],
    "severity": "critical",
    "patterns_checked": 40
  }
}
```

## 🛡️ Security & Blocking

### List Security Rules
```http
GET /api/security-rules
Authorization: Bearer <token>
```

**Response:**
```json
{
  "rules": [
    {
      "id": 1,
      "name": "block-drop-table",
      "pattern": "(?i)drop\\s+table",
      "action": "block",
      "severity": "critical",
      "active": true
    }
  ]
}
```

### Create Security Rule
```http
POST /api/security-rules
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "block-drop-table",
  "pattern": "(?i)drop\\s+table",
  "action": "block",
  "severity": "critical",
  "active": true
}
```

### List Blocked Resources
```http
GET /api/blocked-databases
Authorization: Bearer <token>
```

**Response:**
```json
{
  "blocked_databases": [
    {
      "id": 1,
      "name": "test",
      "type": "database",
      "pattern": "^test$",
      "reason": "Default test database",
      "active": true,
      "created_at": "2024-01-01T00:00:00Z"
    }
  ]
}
```

### Add Blocked Resource
```http
POST /api/blocked-databases
Content-Type: application/json
Authorization: Bearer <token>

{
  "name": "legacy_db_pattern",
  "type": "database",
  "pattern": "^legacy_.*",
  "reason": "Block all legacy databases",
  "active": true
}
```

### Delete Blocked Resource
```http
DELETE /api/blocked-databases/{blocked_id}
Authorization: Bearer <token>
```

### Get Blocking Configuration
```http
GET /api/blocking-config
Authorization: Bearer <token>
```

**Response:**
```json
{
  "blocking_enabled": true,
  "default_blocking": true,
  "custom_blocking": true,
  "total_blocked_resources": 45,
  "active_blocked_resources": 42,
  "blocked_by_type": {
    "databases": 20,
    "users": 15,
    "tables": 7
  }
}
```

### Update Blocking Configuration
```http
PUT /api/blocking-config
Content-Type: application/json
Authorization: Bearer <token>

{
  "blocking_enabled": true,
  "default_blocking": true,
  "custom_blocking": false
}
```

### Seed Default Blocked Resources
```http
POST /api/seed-blocked-resources
Authorization: Bearer <token>
```

**Response:**
```json
{
  "message": "Default blocked resources seeded successfully"
}
```

## 📊 Monitoring & Statistics

### Get System Statistics
```http
GET /api/stats
Authorization: Bearer <token>
```

**Response:**
```json
{
  "total_servers": 5,
  "total_users": 12,
  "total_permissions": 34,
  "total_security_rules": 8,
  "recent_queries": 1234,
  "servers_by_type": {
    "mysql": 2,
    "postgresql": 2,
    "mongodb": 1,
    "mssql": 0,
    "redis": 0
  }
}
```

### Get Audit Log
```http
GET /api/audit-log?limit=100&offset=0
Authorization: Bearer <token>
```

**Response:**
```json
{
  "logs": [
    {
      "id": 1,
      "username": "admin@example.com",
      "action": "create_managed_database",
      "database_name": "production_app",
      "table_name": null,
      "query": "Created database: production_app",
      "result": "success",
      "ip_address": "192.168.1.100",
      "timestamp": "2024-01-01T12:00:00Z"
    }
  ],
  "total": 1234
}
```

### Manual Configuration Sync
```http
POST /api/sync
Authorization: Bearer <token>
```

**Response:**
```json
{
  "message": "Configuration synced to Redis successfully"
}
```

### Health Check
```http
GET /api/health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## ⚠️ Error Handling

### Standard Error Response
```json
{
  "error": true,
  "message": "Detailed error description",
  "code": "ERROR_CODE",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### Common HTTP Status Codes

| Code | Meaning | Description |
|------|---------|-------------|
| `200` | OK | Request successful |
| `201` | Created | Resource created successfully |
| `400` | Bad Request | Invalid request parameters |
| `401` | Unauthorized | Authentication required |
| `403` | Forbidden | Insufficient permissions |
| `404` | Not Found | Resource not found |
| `409` | Conflict | Resource already exists |
| `422` | Unprocessable Entity | Validation failed |
| `500` | Internal Server Error | Server error occurred |

### Validation Error Response
```json
{
  "error": true,
  "message": "Validation failed",
  "code": "VALIDATION_ERROR",
  "details": {
    "field": "database_name",
    "reason": "Database name is required"
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### Security Validation Error Response
```json
{
  "error": true,
  "message": "SQL file failed security validation",
  "code": "SECURITY_VALIDATION_FAILED",
  "details": {
    "severity": "critical",
    "errors": [
      "Shell command detected - Command shell reference: \\bcmd\\b"
    ],
    "warnings": [
      "Potential default/test resource access - Test database access"
    ]
  },
  "timestamp": "2024-01-01T12:00:00Z"
}
```

## 🔒 Security Considerations

### Rate Limiting
- API endpoints are rate-limited to prevent abuse
- Default: 100 requests per minute per user
- Rate limits can be configured per endpoint

### Input Validation
- All input is validated and sanitized
- SQL injection protection at multiple layers
- File upload validation and scanning

### Authentication & Authorization
- Session-based authentication with secure cookies
- Fine-grained permission system
- IP-based access control available

### Audit Logging
- All API calls are logged with full context
- Sensitive data is redacted from logs
- Logs include user, IP, timestamp, and action details

---

*For more detailed information about ArticDBM's architecture and deployment, see the [Architecture Guide](ARCHITECTURE.md) and [Usage Guide](USAGE.md).*