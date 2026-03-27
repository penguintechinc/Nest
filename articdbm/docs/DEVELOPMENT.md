# 🛠️ ArticDBM Development Guide

This guide provides comprehensive instructions for setting up and using the ArticDBM development environment with cluster testing capabilities.

## 📋 Prerequisites

### System Requirements
- **RAM**: 16GB minimum (development stack uses ~15.8GB)
- **CPU**: 4+ cores recommended for optimal performance
- **Storage**: 20GB free space for containers and data
- **OS**: Linux, macOS, or Windows with WSL2

### Required Software
- **Docker**: Version 20.10+ with Docker Compose v2
- **Git**: For cloning and version control
- **Make**: For build automation (optional)
- **Go**: 1.23+ for proxy development (optional)
- **Python**: 3.11+ for manager development (optional)

### Network Requirements
- Ports 3000, 3306-3309, 5432-5436, 6379-6382, 8000-8081, 9090-9093, 16686, 27017-27019
- Internet access for Docker image downloads and threat intelligence feeds

## 🚀 Quick Start

### 1. Clone Repository
```bash
git clone https://github.com/penguintechinc/articdbm.git
cd articdbm
```

### 2. Create Required Configuration Files

#### Test Database Initialization Files

**test/mysql-init.sql**:
```sql
CREATE DATABASE IF NOT EXISTS testapp;
USE testapp;

CREATE TABLE users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    username VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_email (email),
    INDEX idx_username (username)
);

CREATE TABLE products (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    category VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_category (category)
);

INSERT INTO users (email, username) VALUES
    ('john@example.com', 'john_doe'),
    ('jane@example.com', 'jane_smith'),
    ('admin@example.com', 'admin_user');

INSERT INTO products (name, price, category) VALUES
    ('Laptop Pro', 1299.99, 'electronics'),
    ('Office Chair', 299.99, 'furniture'),
    ('Coffee Mug', 19.99, 'kitchen');
```

**test/postgres-init.sql**:
```sql
CREATE DATABASE testapp;
\c testapp;

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    username VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    category VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_products_category ON products(category);

INSERT INTO users (email, username) VALUES
    ('john@example.com', 'john_doe'),
    ('jane@example.com', 'jane_smith'),
    ('admin@example.com', 'admin_user');

INSERT INTO products (name, price, category) VALUES
    ('Laptop Pro', 1299.99, 'electronics'),
    ('Office Chair', 299.99, 'furniture'),
    ('Coffee Mug', 19.99, 'kitchen');
```

**test/mongo-init.js**:
```javascript
db = db.getSiblingDB('testapp');

db.createCollection('users');
db.createCollection('products');

db.users.createIndex({ "email": 1 }, { unique: true });
db.users.createIndex({ "username": 1 });

db.products.createIndex({ "category": 1 });
db.products.createIndex({ "price": 1 });

db.users.insertMany([
    { email: 'john@example.com', username: 'john_doe', created_at: new Date() },
    { email: 'jane@example.com', username: 'jane_smith', created_at: new Date() },
    { email: 'admin@example.com', username: 'admin_user', created_at: new Date() }
]);

db.products.insertMany([
    { name: 'Laptop Pro', price: 1299.99, category: 'electronics', created_at: new Date() },
    { name: 'Office Chair', price: 299.99, category: 'furniture', created_at: new Date() },
    { name: 'Coffee Mug', price: 19.99, category: 'kitchen', created_at: new Date() }
]);
```

#### HAProxy Configuration

**test/haproxy.cfg**:
```
global
    daemon
    log stdout local0 info
    stats socket /tmp/haproxy.sock mode 666 level admin

defaults
    mode tcp
    timeout connect 5s
    timeout client 30s
    timeout server 30s
    log global
    option tcplog
    balance roundrobin

# MySQL load balancing
frontend mysql_frontend
    bind *:3306
    default_backend mysql_backend

backend mysql_backend
    balance roundrobin
    server proxy1 proxy-dev-1:3306 check
    server proxy2 proxy-dev-2:3306 check

# PostgreSQL load balancing
frontend postgres_frontend
    bind *:5432
    default_backend postgres_backend

backend postgres_backend
    balance roundrobin
    server proxy1 proxy-dev-1:5432 check
    server proxy2 proxy-dev-2:5432 check

# MongoDB load balancing
frontend mongo_frontend
    bind *:27017
    default_backend mongo_backend

backend mongo_backend
    balance roundrobin
    server proxy1 proxy-dev-1:27017 check
    server proxy2 proxy-dev-2:27017 check

# Redis load balancing
frontend redis_frontend
    bind *:6379
    default_backend redis_backend

backend redis_backend
    balance roundrobin
    server proxy1 proxy-dev-1:6379 check
    server proxy2 proxy-dev-2:6379 check

# HAProxy statistics
frontend stats
    bind *:9090
    stats enable
    stats uri /stats
    stats refresh 30s
    stats admin if TRUE
```

#### Prometheus Configuration

**test/prometheus.yml**:
```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'articdbm-proxy-1'
    static_configs:
      - targets: ['proxy-dev-1:9091']
    scrape_interval: 5s
    metrics_path: /metrics

  - job_name: 'articdbm-proxy-2'
    static_configs:
      - targets: ['proxy-dev-2:9092']
    scrape_interval: 5s
    metrics_path: /metrics

  - job_name: 'haproxy'
    static_configs:
      - targets: ['haproxy-dev:9090']
    metrics_path: /stats/prometheus
    scrape_interval: 10s

  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 30s
```

#### Grafana Configuration

**test/grafana-datasources.yml**:
```yaml
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    orgId: 1
    url: http://prometheus-dev:9090
    basicAuth: false
    isDefault: true
    editable: true
```

**test/grafana-dashboards.yml**:
```yaml
apiVersion: 1

providers:
  - name: 'default'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    editable: true
    options:
      path: /var/lib/grafana/dashboards
```

### 3. Start Development Environment
```bash
# Create test directory and configuration files
mkdir -p test

# Copy the configuration files above into their respective locations

# Start the full development stack
docker-compose -f docker-compose.dev.yml up -d

# Check status of all services
docker-compose -f docker-compose.dev.yml ps

# View logs from all services
docker-compose -f docker-compose.dev.yml logs -f
```

## 🏗️ Architecture Overview

### Service Layout

The development environment provides a complete ArticDBM cluster with the following components:

#### Core Services
- **redis-dev** (256MB): Configuration and caching backend
- **postgres-dev** (512MB): Manager database storage
- **manager-dev** (1GB): Web UI and API management interface

#### Proxy Cluster
- **proxy-dev-1** (2GB): Primary proxy node with full features
- **proxy-dev-2** (2GB): Secondary proxy node for load balancing
- **haproxy-dev** (256MB): Load balancer for proxy cluster

#### Test Databases
- **mysql-test** (512MB): MySQL 8.0 with sample data
- **postgres-test** (512MB): PostgreSQL 15 with sample data
- **mongo-test** (512MB): MongoDB 7 with sample data
- **redis-test** (256MB): Redis 7 with authentication

#### Monitoring Stack
- **prometheus-dev** (1GB): Metrics collection and storage
- **grafana-dev** (512MB): Visualization and dashboards
- **jaeger-dev** (1GB): Distributed tracing and performance analysis

#### Management Tools
- **redis-insight** (256MB): Redis database management
- **pgadmin-dev** (512MB): PostgreSQL database management
- **adminer-dev** (256MB): Universal database management

### Port Configuration

| Service | Internal Port | External Port | Purpose |
|---------|---------------|---------------|---------|
| **Proxy Services** | | | |
| Proxy Node 1 - MySQL | 3306 | 3306 | Direct MySQL access |
| Proxy Node 1 - PostgreSQL | 5432 | 5432 | Direct PostgreSQL access |
| Proxy Node 1 - MongoDB | 27017 | 27017 | Direct MongoDB access |
| Proxy Node 1 - Redis | 6379 | 6379 | Direct Redis access |
| Proxy Node 1 - Metrics | 9091 | 9091 | Prometheus metrics |
| Proxy Node 2 - MySQL | 3306 | 3308 | Secondary MySQL access |
| Proxy Node 2 - PostgreSQL | 5432 | 5435 | Secondary PostgreSQL access |
| Proxy Node 2 - MongoDB | 27017 | 27018 | Secondary MongoDB access |
| Proxy Node 2 - Redis | 6379 | 6381 | Secondary Redis access |
| Proxy Node 2 - Metrics | 9092 | 9092 | Prometheus metrics |
| **Load Balanced** | | | |
| HAProxy MySQL | 3306 | 3309 | Load balanced MySQL |
| HAProxy PostgreSQL | 5432 | 5436 | Load balanced PostgreSQL |
| HAProxy MongoDB | 27017 | 27019 | Load balanced MongoDB |
| HAProxy Redis | 6379 | 6382 | Load balanced Redis |
| HAProxy Stats | 9090 | 9090 | HAProxy statistics |
| **Test Databases** | | | |
| MySQL Test | 3306 | 3307 | Direct test MySQL |
| PostgreSQL Test | 5432 | 5434 | Direct test PostgreSQL |
| MongoDB Test | 27017 | 27017 | Direct test MongoDB |
| Redis Test | 6379 | 6380 | Direct test Redis |
| **Management** | | | |
| Manager UI/API | 8000 | 8000 | Web interface |
| Redis Config | 6379 | 6379 | Config backend |
| PostgreSQL Manager | 5432 | 5433 | Manager database |
| **Monitoring** | | | |
| Prometheus | 9090 | 9093 | Metrics collection |
| Grafana | 3000 | 3000 | Dashboards |
| Jaeger | 16686 | 16686 | Tracing UI |
| **Database Tools** | | | |
| Redis Insight | 8001 | 8001 | Redis management |
| pgAdmin | 80 | 8080 | PostgreSQL management |
| Adminer | 8080 | 8081 | Universal DB tool |

## 🔧 Development Workflows

### Testing Proxy Functionality

#### Connection Testing
```bash
# Test MySQL through primary proxy
mysql -h localhost -P 3306 -u testuser -ptestpass123 -D testapp

# Test MySQL through secondary proxy
mysql -h localhost -P 3308 -u testuser -ptestpass123 -D testapp

# Test MySQL through load balancer
mysql -h localhost -P 3309 -u testuser -ptestpass123 -D testapp

# Test PostgreSQL through primary proxy
psql -h localhost -p 5432 -U testuser -d testapp

# Test PostgreSQL through secondary proxy
psql -h localhost -p 5435 -U testuser -d testapp

# Test PostgreSQL through load balancer
psql -h localhost -p 5436 -U testuser -d testapp
```

#### Security Testing
```bash
# Test SQL injection detection
mysql -h localhost -P 3306 -u testuser -ptestpass123 -D testapp \
  -e "SELECT * FROM users WHERE id = 1 OR 1=1;"

# Test blocked default database access
mysql -h localhost -P 3306 -u root -ptestroot123 \
  -e "USE mysql; SHOW TABLES;"

# Test shell command blocking
mysql -h localhost -P 3306 -u testuser -ptestpass123 -D testapp \
  -e "SELECT * FROM users; xp_cmdshell('dir');"
```

### Performance Testing

#### Load Testing with mysqlslap
```bash
# Basic load test through proxy
mysqlslap --host=localhost --port=3306 \
  --user=testuser --password=testpass123 \
  --database=testapp \
  --concurrency=10 --iterations=100 \
  --auto-generate-sql \
  --auto-generate-sql-add-autoincrement \
  --auto-generate-sql-load-type=mixed

# Load test through load balancer
mysqlslap --host=localhost --port=3309 \
  --user=testuser --password=testpass123 \
  --database=testapp \
  --concurrency=50 --iterations=200 \
  --auto-generate-sql
```

#### PostgreSQL Benchmarking
```bash
# Initialize pgbench
pgbench -h localhost -p 5432 -U testuser -d testapp -i

# Run benchmark through proxy
pgbench -h localhost -p 5432 -U testuser -d testapp \
  -c 10 -j 2 -t 1000

# Run benchmark through load balancer
pgbench -h localhost -p 5436 -U testuser -d testapp \
  -c 20 -j 4 -t 2000
```

### Monitoring and Observability

#### Prometheus Metrics
Access metrics at:
- Proxy Node 1: http://localhost:9091/metrics
- Proxy Node 2: http://localhost:9092/metrics
- Prometheus UI: http://localhost:9093

Key metrics to monitor:
```promql
# Query rate across both nodes
sum(rate(articdbm_total_queries[5m]))

# Connection count by node
articdbm_active_connections

# Error rates
rate(articdbm_auth_failures_total[5m])
rate(articdbm_sql_injection_attempts_total[5m])

# Query latency percentiles
histogram_quantile(0.95, sum(rate(articdbm_query_duration_seconds_bucket[5m])) by (le))
```

#### Grafana Dashboards
Access Grafana at: http://localhost:3000
- Username: admin
- Password: devpass123

Pre-configured dashboards include:
- ArticDBM Overview
- Proxy Performance
- Security Events
- Database Health

#### Distributed Tracing
Access Jaeger at: http://localhost:16686

Trace key operations:
- Query execution paths
- Authentication flows
- Security validation
- Cache operations

### Log Analysis

#### Container Logs
```bash
# View all logs
docker-compose -f docker-compose.dev.yml logs -f

# View proxy logs only
docker-compose -f docker-compose.dev.yml logs -f proxy-dev-1 proxy-dev-2

# View manager logs
docker-compose -f docker-compose.dev.yml logs -f manager-dev

# View specific timeframe
docker-compose -f docker-compose.dev.yml logs --since="1h" proxy-dev-1
```

#### Application Logs
```bash
# Proxy logs are available in mounted volumes
docker exec -it articdbm-proxy-dev-1 tail -f /app/logs/proxy.log

# Manager logs
docker exec -it articdbm-manager-dev tail -f /app/logs/manager.log
```

## 🐛 Development and Debugging

### Local Development Setup

#### Proxy Development
```bash
# Work on proxy code locally
cd services/proxy

# Install dependencies
go mod tidy

# Run tests
go test ./...

# Build locally
go build -o articdbm-proxy .

# Run with development config
REDIS_ADDR=localhost:6379 \
LOG_LEVEL=debug \
./articdbm-proxy
```

#### Manager Development
```bash
# Work on manager code locally
cd services/manager

# Create virtual environment
python -m venv venv
source venv/bin/activate

# Install dependencies
pip install -r requirements.txt

# Run development server
python -m py4web run apps/
```

### Debugging Common Issues

#### Connection Problems
```bash
# Check container status
docker-compose -f docker-compose.dev.yml ps

# Check port bindings
netstat -tlnp | grep -E "(3306|5432|6379|8000)"

# Test Redis connectivity
redis-cli -h localhost -p 6379 ping

# Test database connectivity
docker exec -it articdbm-mysql-test mysql -u testuser -ptestpass123 -e "SELECT 1"
```

#### Performance Issues
```bash
# Monitor resource usage
docker stats

# Check proxy metrics
curl http://localhost:9091/metrics | grep articdbm_

# Check Redis performance
redis-cli -h localhost -p 6379 info stats

# Monitor query patterns
docker exec -it articdbm-proxy-dev-1 tail -f /app/logs/audit.log
```

#### Memory Management
```bash
# Check memory usage per container
docker stats --format "table {{.Container}}\t{{.MemUsage}}\t{{.MemPerc}}"

# Force garbage collection in Go services
curl -X POST http://localhost:9091/debug/gc

# Monitor Redis memory usage
redis-cli -h localhost -p 6379 info memory
```

## 🔒 Security Testing

### Threat Intelligence Testing
```bash
# Add test threat indicators via manager API
curl -X POST http://localhost:8000/api/threat-intelligence \
  -H "Content-Type: application/json" \
  -d '{
    "type": "ip",
    "value": "192.168.1.100",
    "confidence": 85,
    "source": "manual_test"
  }'

# Test blocking functionality
curl -H "X-Forwarded-For: 192.168.1.100" http://localhost:3306
```

### Authentication Testing
```bash
# Test API key authentication
curl -H "X-API-Key: invalid-key" http://localhost:8000/api/servers

# Test session-based access
curl -c cookies.txt -d "email=admin@example.com&password=admin123" \
  http://localhost:8000/auth/login

curl -b cookies.txt http://localhost:8000/api/users
```

### SQL Injection Testing
Use the provided test cases in the proxy security module:
```bash
# Run security tests
cd services/proxy
go test ./internal/security/... -v

# Test specific patterns
mysql -h localhost -P 3306 -u testuser -ptestpass123 -D testapp \
  -e "SELECT * FROM users WHERE id = '1' UNION SELECT NULL,@@version,NULL,NULL--;"
```

## 📊 Performance Optimization

### Resource Tuning

#### Database Optimization
```sql
-- MySQL performance tuning
SET GLOBAL innodb_buffer_pool_size = 268435456;  -- 256MB
SET GLOBAL query_cache_size = 33554432;          -- 32MB
SET GLOBAL max_connections = 200;

-- PostgreSQL optimization
ALTER SYSTEM SET shared_buffers = '128MB';
ALTER SYSTEM SET effective_cache_size = '256MB';
ALTER SYSTEM SET maintenance_work_mem = '32MB';
SELECT pg_reload_conf();
```

#### Proxy Configuration
```bash
# Optimize connection pools via environment variables
export MAX_CONNECTIONS=1000
export CONNECTION_LIFETIME=180s
export IDLE_TIMEOUT=30s
export CONNECTION_WARMUP_PERCENTAGE=30

# Restart proxy with new config
docker-compose -f docker-compose.dev.yml restart proxy-dev-1 proxy-dev-2
```

### Load Testing Scenarios

#### Concurrent User Simulation
```bash
# Simulate multiple users
for i in {1..10}; do
  mysql -h localhost -P 3309 -u testuser -ptestpass123 -D testapp \
    -e "SELECT COUNT(*) FROM users; SELECT COUNT(*) FROM products;" &
done
wait

# Monitor during load
watch -n 1 'curl -s http://localhost:9091/metrics | grep articdbm_active_connections'
```

#### Cache Performance Testing
```bash
# Test query caching
for i in {1..100}; do
  mysql -h localhost -P 3306 -u testuser -ptestpass123 -D testapp \
    -e "SELECT * FROM users WHERE email = 'john@example.com';"
done

# Monitor cache hit rates
curl http://localhost:9091/metrics | grep cache_hit_ratio
```

## 🚀 Advanced Features Testing

### ML Query Optimization
```bash
# Enable ML optimization
docker exec -it articdbm-proxy-dev-1 \
  curl -X POST http://localhost:9091/api/ml/enable

# Run queries to collect performance data
mysql -h localhost -P 3306 -u testuser -ptestpass123 -D testapp \
  -e "SELECT u.*, p.* FROM users u JOIN products p ON u.id = p.id;"

# Check optimization recommendations
curl http://localhost:9091/api/ml/recommendations
```

### Blue/Green Deployment Testing
```bash
# Simulate deployment scenario
curl -X POST http://localhost:8000/api/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "type": "blue_green",
    "traffic_split": 90,
    "target_backend": "green"
  }'

# Monitor traffic distribution
curl http://localhost:9090/stats
```

### Threat Intelligence Integration
```bash
# Test STIX feed processing
curl -X POST http://localhost:8000/api/threat-feeds \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/stix-feed.json",
    "format": "stix",
    "schedule": "hourly"
  }'

# Check threat status
curl http://localhost:8000/api/threats/status
```

## 🔧 Maintenance and Troubleshooting

### Health Checks
```bash
# Overall system health
curl http://localhost:8000/api/health

# Individual proxy health
curl http://localhost:9091/health
curl http://localhost:9092/health

# Database connectivity
curl http://localhost:8000/api/servers/health

# Cache status
redis-cli -h localhost -p 6379 ping
```

### Backup and Recovery
```bash
# Backup development data
docker exec articdbm-postgres-dev pg_dump -U articdbm articdbm > backup.sql
docker exec articdbm-mysql-test mysqldump -u root -ptestroot123 --all-databases > mysql_backup.sql

# Restore from backup
docker exec -i articdbm-postgres-dev psql -U articdbm articdbm < backup.sql
```

### Log Rotation and Cleanup
```bash
# Clean old logs
docker exec articdbm-proxy-dev-1 find /app/logs -name "*.log" -mtime +7 -delete

# Rotate logs
docker exec articdbm-proxy-dev-1 logrotate /etc/logrotate.conf

# Clean Docker resources
docker system prune -f
docker volume prune -f
```

## 📚 Additional Resources

### Management Interfaces
- **ArticDBM Manager**: http://localhost:8000 (admin/devpass123)
- **HAProxy Stats**: http://localhost:9090/stats
- **Grafana**: http://localhost:3000 (admin/devpass123)
- **Prometheus**: http://localhost:9093
- **Jaeger**: http://localhost:16686
- **Redis Insight**: http://localhost:8001
- **pgAdmin**: http://localhost:8080 (admin@articdbm.dev/devpass123)
- **Adminer**: http://localhost:8081

### API Documentation
- **Manager API**: http://localhost:8000/api/docs
- **Proxy Metrics**: http://localhost:9091/metrics
- **Health Endpoints**: http://localhost:8000/api/health

### Configuration Files
- **Proxy Config**: `services/proxy/internal/config/config.go`
- **Manager Config**: `services/manager/app.py`
- **Docker Compose**: `docker-compose.dev.yml`

## 🎯 Next Steps

1. **Start Development**: Use this environment for ArticDBM development
2. **Add Tests**: Implement unit and integration tests
3. **Performance Tuning**: Optimize based on your use case
4. **Custom Features**: Add organization-specific features
5. **Production Deployment**: Adapt for production environment

---

*This development environment provides a complete ArticDBM v1.2.0 "Arctic Storm" testing platform with enterprise features including XDP acceleration (disabled for compatibility), threat intelligence, ML optimization, and comprehensive monitoring.*