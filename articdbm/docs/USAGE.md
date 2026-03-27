# 📘 ArticDBM v1.2.0 Usage Guide

This comprehensive guide covers installation, configuration, and usage of ArticDBM v1.2.0 "Arctic Storm" - the world's first XDP-accelerated enterprise database proxy with advanced security and AI-driven operations.

## 📦 Installation

### Docker Compose (Recommended)

```bash
# Clone repository
git clone https://github.com/penguintechinc/articdbm.git
cd articdbm

# Start services
docker-compose up -d

# Check status
docker-compose ps
```

### Kubernetes with XDP Support

For optimal performance with XDP acceleration:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: articdbm-proxy
spec:
  replicas: 3
  selector:
    matchLabels:
      app: articdbm-proxy
  template:
    metadata:
      labels:
        app: articdbm-proxy
    spec:
      containers:
      - name: proxy
        image: articdbm/proxy:1.2.0
        env:
        - name: REDIS_ADDR
          value: "redis-service:6379"
        - name: XDP_ENABLED
          value: "true"
        - name: NUMA_OPTIMIZATION
          value: "true"
        - name: THREAT_INTEL_ENABLED
          value: "true"
        securityContext:
          privileged: true  # Required for XDP
          capabilities:
            add: ["NET_ADMIN", "SYS_ADMIN"]
        ports:
        - containerPort: 3306
        - containerPort: 5432
        - containerPort: 9090  # Metrics
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
      nodeSelector:
        xdp-capable: "true"  # Ensure XDP-capable nodes
```

### Kubernetes Operator (Recommended for Production)

```yaml
apiVersion: articdbm.io/v1alpha1
kind: ArticDBMProxy
metadata:
  name: production-proxy
spec:
  replicas: 5
  xdp:
    enabled: true
    numWorkers: 4
  security:
    threatIntelligence: true
    complianceScanning: true
  resources:
    requests:
      memory: "1Gi"
      cpu: "1000m"
    limits:
      memory: "4Gi"
      cpu: "4000m"
```

## ⚙️ Configuration

### Environment Variables

#### Proxy Configuration

| Variable | Description | Default | v1.2.0 |
|----------|-------------|---------|--------|
| `REDIS_ADDR` | Redis connection string | `localhost:6379` | ✅ |
| `MYSQL_ENABLED` | Enable MySQL proxy | `true` | ✅ |
| `MYSQL_PORT` | MySQL proxy port | `3306` | ✅ |
| `POSTGRESQL_ENABLED` | Enable PostgreSQL proxy | `true` | ✅ |
| `POSTGRESQL_PORT` | PostgreSQL proxy port | `5432` | ✅ |
| `SQL_INJECTION_DETECTION` | Enable SQL injection detection | `true` | ✅ |
| `MAX_CONNECTIONS` | Maximum connections per backend | `1000` | ✅ |
| `TLS_ENABLED` | Enable TLS support | `false` | ✅ |
| `CLUSTER_MODE` | Enable cluster mode | `false` | ✅ |
| **XDP_ENABLED** | Enable XDP acceleration | `false` | 🆕 |
| **NUMA_OPTIMIZATION** | Enable NUMA optimization | `true` | 🆕 |
| **XDP_WORKERS** | Number of XDP worker threads | `4` | 🆕 |
| **THREAT_INTEL_ENABLED** | Enable threat intelligence | `true` | 🆕 |
| **SECURITY_SCANNING** | Enable daily security scans | `true` | 🆕 |
| **ML_OPTIMIZATION** | Enable ML query optimization | `true` | 🆕 |
| **TRACING_ENABLED** | Enable distributed tracing | `false` | 🆕 |
| **BACKUP_ENABLED** | Enable automated backups | `true` | 🆕 |

#### Manager Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgresql://articdbm:articdbm@postgres/articdbm` |
| `REDIS_HOST` | Redis host | `redis` |
| `REDIS_PORT` | Redis port | `6379` |
| `SESSION_SECRET` | Session encryption key | Auto-generated |

## 👥 User Management

### Creating Users

1. Access the manager UI: `http://localhost:8000`
2. Navigate to **Users** → **Add User**
3. Fill in user details:
   - Email (username)
   - Password
   - Enable/disable status

### Managing Permissions

Permissions control database and table access:

```json
{
  "user_id": "john@example.com",
  "database": "production_db",
  "table": "users",
  "actions": ["read", "write"]
}
```

#### Permission Levels

- **Database**: `*` for all databases or specific database name
- **Table**: `*` for all tables or specific table name
- **Actions**: 
  - `read`: SELECT queries
  - `write`: INSERT, UPDATE, DELETE
  - `*`: All operations

## 🗃️ Database Management

ArticDBM now provides comprehensive database lifecycle management capabilities through its enhanced manager interface.

### Managing Databases

#### Adding Managed Databases

Via Manager UI:
1. Navigate to **Databases** → **Add Database**
2. Configure database details:
   - Name and description
   - Associated server
   - Database name on server
   - Auto-backup settings
   - Backup schedule

Via API:
```bash
curl -X POST http://localhost:8000/api/databases \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production_app",
    "server_id": 1,
    "database_name": "prod_db",
    "description": "Main production database",
    "auto_backup": true,
    "backup_schedule": "0 2 * * *"
  }'
```

#### Database Operations

- **Create**: Add new databases to management
- **Update**: Modify database configurations and settings
- **Delete**: Remove databases from management (soft delete)
- **Schema Management**: Track and version database schemas
- **Backup Configuration**: Automated backup scheduling

### SQL File Management

ArticDBM provides secure SQL file upload and execution capabilities with comprehensive security validation.

#### Uploading SQL Files

Via Manager UI:
1. Navigate to **Databases** → Select Database → **SQL Files**
2. Upload SQL file with metadata:
   - File type: `init`, `backup`, `migration`, `patch`
   - Security validation occurs automatically
   - Syntax checking performed

Via API:
```bash
curl -X POST http://localhost:8000/api/sql-files \
  -H "Content-Type: application/json" \
  -d '{
    "name": "user_table_migration.sql",
    "database_id": 1,
    "file_type": "migration",
    "file_content": "CREATE TABLE users (id INT PRIMARY KEY, email VARCHAR(255));"
  }'
```

#### SQL File Security Validation

Every SQL file undergoes comprehensive security analysis:

**Dangerous Pattern Detection:**
- SQL injection patterns
- System command execution attempts
- File system access operations
- Database introspection queries
- Destructive operations

**Shell Command Protection:**
- Command shell references (`cmd`, `powershell`, `bash`)
- System function calls
- File system operations
- Process execution attempts

**Default Resource Protection:**
- Access to system databases
- Use of default administrative accounts
- Test/demo database operations

#### SQL File Execution

```bash
# Execute validated SQL file
curl -X POST http://localhost:8000/api/sql-files/1/execute \
  -H "Content-Type: application/json"

# Re-validate existing file
curl -X POST http://localhost:8000/api/sql-files/1/validate \
  -H "Content-Type: application/json"
```

**Execution Requirements:**
- File must pass security validation
- Database must be active and accessible
- User must have appropriate permissions
- File cannot have been previously executed

## 🗄️ Database Backend Configuration

### Adding Database Servers

Via API:
```bash
curl -X POST http://localhost:8000/api/servers \
  -H "Content-Type: application/json" \
  -d '{
    "name": "mysql-primary",
    "type": "mysql",
    "host": "192.168.1.10",
    "port": 3306,
    "username": "root",
    "password": "secret",
    "database": "myapp",
    "role": "write",
    "weight": 1,
    "tls_enabled": false
  }'
```

### Read/Write Splitting

Configure multiple backends with different roles:

```yaml
backends:
  - name: mysql-primary
    role: write
    host: primary.example.com
  - name: mysql-replica-1
    role: read
    host: replica1.example.com
    weight: 2
  - name: mysql-replica-2
    role: read
    host: replica2.example.com
    weight: 1
```

## 🔒 Security Configuration

ArticDBM provides multiple layers of security protection to safeguard your databases against various attack vectors.

### Enhanced SQL Injection Detection

Enable comprehensive pattern-based detection:

```bash
SQL_INJECTION_DETECTION=true
```

ArticDBM detects **40+ attack patterns** including:
- Classic SQL injection (`OR 1=1`, `UNION SELECT`)
- Blind SQL injection techniques
- Time-based injection (`WAITFOR DELAY`, `SLEEP`)
- Error-based injection (`UPDATEXML`, `EXTRACTVALUE`)
- Stacked queries and batch operations
- System information disclosure attempts

### Shell Command Attack Protection

ArticDBM provides advanced protection against shell command injection and system-level attacks:

**Protected Command Categories:**
- **System Commands**: `cmd`, `powershell`, `bash`, `sh`
- **File System Operations**: `chmod`, `chown`, `mkdir`, `rm -rf`
- **Process Management**: `kill`, `killall`, `ps`, `top`
- **Network Operations**: `wget`, `curl`, `nc`, `netcat`
- **Text Processing**: `awk`, `sed`, `grep`, `find`
- **System Administration**: `su`, `sudo`, `wmic`, `reg`

**Detection Examples:**
```bash
# These queries would be blocked
SELECT * FROM users WHERE id = 1; DROP TABLE users; --
SELECT * FROM products WHERE name = 'test'; xp_cmdshell 'dir'
INSERT INTO logs VALUES ('data', system('cat /etc/passwd'))
```

### Default Database/Account Blocking

ArticDBM automatically blocks access to common default and test resources:

**Blocked System Databases:**
- SQL Server: `master`, `msdb`, `tempdb`, `model`
- MySQL: `mysql`, `sys`, `information_schema`, `performance_schema`
- PostgreSQL: `postgres`, `template0`, `template1`
- MongoDB: `admin`, `local`, `config`
- Common test DBs: `test`, `sample`, `demo`, `example`

**Blocked Default Accounts:**
- Administrative: `sa`, `root`, `admin`, `administrator`
- Service accounts: `mysql`, `postgres`, `oracle`, `sqlserver`
- Test accounts: `test`, `demo`, `sample`, `user`, `guest`
- Anonymous: empty usernames, `anonymous`

#### Managing Blocked Resources

Via Manager UI:
1. Navigate to **Security** → **Blocked Resources**
2. View current blocking rules
3. Add custom patterns
4. Enable/disable specific rules

Via API:
```bash
# Add custom blocked database pattern
curl -X POST http://localhost:8000/api/blocked-databases \
  -H "Content-Type: application/json" \
  -d '{
    "name": "legacy_db_pattern",
    "type": "database",
    "pattern": "^legacy_.*",
    "reason": "Block all legacy databases",
    "active": true
  }'

# Block specific username pattern
curl -X POST http://localhost:8000/api/blocked-databases \
  -H "Content-Type: application/json" \
  -d '{
    "name": "temp_user_pattern",
    "type": "username",
    "pattern": "^temp_.*",
    "reason": "Block temporary user accounts",
    "active": true
  }'
```

### Custom Security Rules

Add custom security rules via the manager:

```json
{
  "name": "block-drop-table",
  "pattern": "(?i)drop\\s+table",
  "action": "block",
  "severity": "critical"
}
```

**Security Rule Actions:**
- `block`: Immediately reject the query
- `alert`: Allow but generate security alert
- `log`: Allow but log for audit

**Severity Levels:**
- `critical`: System-level threats, shell commands
- `high`: SQL injection, destructive operations
- `medium`: Suspicious patterns, default resource access
- `low`: Minor policy violations

### TLS Configuration

Enable TLS for proxy connections:

```bash
TLS_ENABLED=true
TLS_CERT=/path/to/cert.pem
TLS_KEY=/path/to/key.pem
```

## 📊 Monitoring

### Prometheus Metrics

Metrics available at `http://localhost:9090/metrics`:

- `articdbm_active_connections` - Active connection count
- `articdbm_total_queries` - Total queries processed
- `articdbm_query_duration_seconds` - Query execution time
- `articdbm_auth_failures_total` - Authentication failures
- `articdbm_sql_injection_attempts_total` - Blocked SQL injections

### Grafana Dashboard

Import the provided dashboard:

```json
{
  "dashboard": {
    "title": "ArticDBM Monitoring",
    "panels": [
      {
        "title": "Query Rate",
        "targets": [
          {
            "expr": "rate(articdbm_total_queries[5m])"
          }
        ]
      }
    ]
  }
}
```

## 🔄 High Availability

### Cluster Mode

Enable cluster mode for multiple proxy instances:

```bash
CLUSTER_MODE=true
CLUSTER_REDIS_ADDR=redis-cluster:6379
```

### Load Balancing

Use a network load balancer:

```nginx
upstream articdbm_mysql {
    server proxy1:3306;
    server proxy2:3306;
    server proxy3:3306;
}
```

## 🧪 Testing

### Connection Test

MySQL:
```bash
mysql -h localhost -P 3306 -u testuser -p
> SHOW DATABASES;
```

PostgreSQL:
```bash
psql -h localhost -p 5432 -U testuser -d testdb
\l
```

### Performance Test

```bash
# MySQL benchmark
mysqlslap --host=localhost --port=3306 \
  --user=testuser --password=testpass \
  --concurrency=50 --iterations=100 \
  --auto-generate-sql

# PostgreSQL benchmark
pgbench -h localhost -p 5432 -U testuser \
  -c 10 -j 2 -t 1000 testdb
```

## 🐛 Troubleshooting

### Common Issues

#### Connection Refused
- Check if proxy is running: `docker ps`
- Verify port bindings: `netstat -tlnp`
- Check firewall rules

#### Authentication Failed
- Verify user exists in manager
- Check permissions for database/table
- Review audit logs

#### Slow Queries
- Check backend server performance
- Review connection pool settings
- Enable query caching

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug
```

View logs:
```bash
docker-compose logs -f proxy
docker-compose logs -f manager
```

## 📝 Examples

### Python Connection

```python
import pymysql

connection = pymysql.connect(
    host='localhost',
    port=3306,
    user='app_user',
    password='secret',
    database='myapp'
)

with connection.cursor() as cursor:
    cursor.execute("SELECT * FROM users")
    results = cursor.fetchall()
```

### Node.js Connection

```javascript
const mysql = require('mysql2');

const connection = mysql.createConnection({
  host: 'localhost',
  port: 3306,
  user: 'app_user',
  password: 'secret',
  database: 'myapp'
});

connection.query('SELECT * FROM users', (err, results) => {
  console.log(results);
});
```

### Go Connection

```go
import (
    "database/sql"
    _ "github.com/go-sql-driver/mysql"
)

db, err := sql.Open("mysql", "app_user:secret@tcp(localhost:3306)/myapp")
if err != nil {
    panic(err)
}
defer db.Close()

rows, err := db.Query("SELECT * FROM users")
```

---
*For more information, see the [Architecture Guide](ARCHITECTURE.md) or [Cloudflare Setup Guide](CLOUDFLARE-SETUP.md).*