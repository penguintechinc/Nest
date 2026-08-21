# NEST Administration Guide

This guide covers deployment, configuration, monitoring, and troubleshooting for NEST infrastructure platform administrators.

## Architecture Overview

NEST consists of four services:

| Service            | Language                | Port    | Purpose                                                |
| ------------------ | ----------------------- | ------- | ------------------------------------------------------ |
| **API Gateway**    | Go                      | 8080    | JWT auth, RBAC middleware, REST API routing            |
| **Manager**        | Python (Quart)          | 5000    | 18 blueprints, background workers, resource connectors |
| **WebUI**          | React (Vite/TypeScript) | 80/3000 | Browser-based management console                       |
| **K8s Controller** | Go                      | --      | Reconciliation loop, event watcher                     |

### Inter-Service Communication

- WebUI communicates with the API Gateway via REST over HTTPS
- API Gateway communicates with the Manager via gRPC (port 50051) or REST over HTTP/2
- Manager communicates with Kubernetes API, external database connectors, and backup backends
- K8s Controller watches Kubernetes resources and reconciles desired state

---

## 1. Prerequisites

| Component  | Version | Notes                                               |
| ---------- | ------- | --------------------------------------------------- |
| Go         | 1.24+   | API Gateway and K8s Controller                      |
| Python     | 3.13+   | Manager (Quart async framework)                     |
| Node.js    | 18+     | WebUI build tooling (Vite)                          |
| PostgreSQL | 16.x    | Primary database (`postgres:16-bookworm`)           |
| Redis      | 7.x     | Cache and session store (`redis:7-bookworm`)        |
| Kubernetes | 1.28+   | MicroK8s (local), managed cluster (beta/prod)       |
| Docker     | 24+     | Container builds (Debian bookworm-slim base images) |
| Helm       | 3.x     | Beta/prod deployments                               |
| kubectl    | 1.28+   | Cluster management                                  |

**Local development additionally requires:**

- MicroK8s with registry enabled (`microk8s enable registry`)
- `golangci-lint` for Go linting
- `flake8`, `black`, `isort`, `mypy`, `bandit` for Python linting

---

## 2. Installation

```bash
# Clone repository
git clone https://github.com/penguintechinc/nest.git
cd nest

# Install Python dependencies (Manager)
cd apps/manager && pip install -r requirements.txt && cd ../..

# Install Node.js dependencies (WebUI)
cd web && npm ci && cd ..

# Install Go dependencies (API Gateway, K8s Controller)
go mod download

# Build container images for local alpha
docker build -t localhost:32000/nest-api:latest -f Dockerfile.api .
docker build -t localhost:32000/nest-manager:latest -f apps/manager/Dockerfile apps/manager/
docker build -t localhost:32000/nest-web:latest -f web/Dockerfile web/

# Push to MicroK8s registry
docker push localhost:32000/nest-api:latest
docker push localhost:32000/nest-manager:latest
docker push localhost:32000/nest-web:latest
```

---

## 3. Kubernetes Deployment

### Alpha (Kustomize -- Local Development)

```bash
# Deploy to local MicroK8s cluster
kubectl apply --context local-alpha -k k8s/kustomize/overlays/alpha

# Verify
kubectl --context local-alpha get pods -n nest

# Remove
kubectl delete --context local-alpha -k k8s/kustomize/overlays/alpha
```

### Beta (Helm)

```bash
# Tag and push images to beta registry
docker tag nest-manager:latest registry-dal2.penguintech.cloud/nest/nest-manager:beta-$(date +%s)
docker push registry-dal2.penguintech.cloud/nest/nest-manager:beta-*

# Deploy
helm upgrade --install nest ./k8s/helm/nest \
  --kube-context dal2-beta \
  --namespace nest --create-namespace \
  --values ./k8s/helm/nest/values-beta.yaml

# Verify
kubectl --context dal2-beta rollout status deployment/nest-manager -n nest
```

### Production (Helm)

```bash
helm upgrade --install nest ./k8s/helm/nest \
  --kube-context nest-prod \
  --namespace nest --create-namespace \
  --values ./k8s/helm/nest/values-prod.yaml
```

### Rollback

```bash
helm rollback nest 1 --kube-context dal2-beta --namespace nest
```

### Environment Domains

| Environment | Domain                           | Context       |
| ----------- | -------------------------------- | ------------- |
| Alpha       | `https://nest.localhost.local`   | `local-alpha` |
| Beta        | `https://nest.penguintech.cloud` | `dal2-beta`   |
| Production  | `https://nest.penguincloud.io`   | `nest-prod`   |

---

## 4. Configuration

### Database Configuration

```bash
DB_TYPE=postgresql        # postgresql | mysql | sqlite
DB_HOST=localhost
DB_PORT=5432
DB_NAME=nest_db
DB_USER=nest-manager-rw   # Per-service account (mandatory)
DB_PASS=<secret>
DB_POOL_SIZE=10
DB_MAX_RETRIES=5
DB_RETRY_DELAY=5
```

Each service (Manager, API Gateway) must have its own dedicated database credentials.

### Application Configuration

```bash
# JWT
JWT_SECRET_KEY=<secret>
JWT_ACCESS_TOKEN_EXPIRES=3600    # 1 hour

# License Server
LICENSE_KEY=PENG-XXXX-XXXX-XXXX-XXXX-ABCD
LICENSE_SERVER_URL=https://license.penguintech.io
PRODUCT_NAME=nest
RELEASE_MODE=false               # true for production

# Logging
LOG_LEVEL=info                   # debug | info | warn | error

# Kubernetes
KUBECONFIG=/path/to/kubeconfig
K8S_NAMESPACE=nest
```

### Cache Configuration

```bash
CACHE_HOST=localhost
CACHE_PORT=6379
CACHE_USER=nest-manager          # Per-service Redis/Valkey account
CACHE_PASS=<secret>
```

---

## 4.1 Connecting Cloud Accounts

NEST can provision and adopt resources in public cloud environments. Cloud credentials are stored as Kubernetes Secrets and referenced by DataResources.

### Creating Cloud Credential Secrets

Each cloud provider requires its own Secret with provider-specific keys. Create the Secret in the same namespace as the DataResource.

**AWS (EBS volumes and S3 buckets):**

```bash
kubectl create secret generic aws-credentials \
  --from-literal=access_key_id=<your-access-key> \
  --from-literal=secret_access_key=<your-secret-key> \
  --from-literal=session_token=<optional-session-token> \
  -n default
```

**GCP (Persistent Disk and GCS):**

```bash
kubectl create secret generic gcp-credentials \
  --from-file=service_account_json=<path-to-service-account.json> \
  -n default
```

GCP also supports Application Default Credentials, which use the pod's default service account if the Secret is omitted.

**Azure (Managed Disk and Blob Storage):**

```bash
kubectl create secret generic azure-credentials \
  --from-literal=client_id=<your-client-id> \
  --from-literal=client_secret=<your-client-secret> \
  --from-literal=tenant_id=<your-tenant-id> \
  -n default
```

**DigitalOcean (block volumes):**

```bash
kubectl create secret generic do-credentials \
  --from-literal=do_token=<your-api-token> \
  -n default
```

**Vultr (block volumes):**

```bash
kubectl create secret generic vultr-credentials \
  --from-literal=vultr_api_key=<your-api-key> \
  -n default
```

**Linode (block volumes):**

```bash
kubectl create secret generic linode-credentials \
  --from-literal=linode_token=<your-api-token> \
  -n default
```

### Cloud Provider Permissions

Each cloud provider requires specific permissions to provision and manage resources:

**AWS:**

- For EBS volumes: EC2 `CreateVolume`, `DeleteVolume`, `DescribeVolumes`, `CreateTags`
- For S3 buckets: S3 `CreateBucket`, `DeleteBucket`, `GetBucketPolicy`, `PutBucketPolicy`
- For RDS adoption: RDS `DescribeDBInstances`, `DescribeDBClusters` (read-only, for enrichment)

**GCP:**

- For Persistent Disk: Compute `compute.disks.create`, `compute.disks.delete`, `compute.disks.get`
- For GCS buckets: Storage `storage.buckets.create`, `storage.buckets.delete`, `storage.objects.list`
- For Cloud SQL adoption: Cloud SQL `cloudsql.instances.list` (read-only, for enrichment)

**Azure:**

- For Managed Disk: `Microsoft.Compute/disks/write`, `Microsoft.Compute/disks/delete`
- For Blob Storage: `Microsoft.Storage/storageAccounts/blobServices/containers/write`, `Microsoft.Storage/storageAccounts/blobServices/containers/delete`
- For Azure Database adoption: `Microsoft.DBforPostgreSQL/servers/read` (read-only, for enrichment)

**DigitalOcean, Vultr, Linode:**

- Full API access to manage block volumes (token-based authentication; granular scopes depend on provider)

### Registering Credentials in NEST

Once a Secret is created, reference it in a DataResource via:

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: cloud-backed-volume
  namespace: default
spec:
  type: block
  size: 500Gi
  external:
    provider: aws # aws | gcp | azure | do | vultr | linode
    region: us-east-1 # Provider-specific region
    credentialSecret: aws-credentials # Name of Secret in same namespace
    extra:
      # Optional provider-specific config for Azure:
      subscription_id: <your-subscription-id>
      resource_group: <your-resource-group>
      storage_account_name: <your-storage-account>
```

For adopted resources, reference credentials similarly:

```yaml
spec:
  type: database
  external:
    provider: aws
    credentialSecret: aws-credentials
  import:
    connectionString: postgresql://host:5432/mydb
```

---

## 5. Database Setup

NEST uses the dual-library architecture:

- **SQLAlchemy + Alembic**: Schema definition and migrations only
- **PyDAL**: All runtime database operations (`migrate=False` always)

### Initial Schema Setup

SQLAlchemy models in `apps/manager/db_models/` define the schema. On first deployment, `Base.metadata.create_all()` runs at startup (idempotent).

### Running Migrations

Alembic migrations must be run manually -- never at application startup:

```bash
cd apps/manager

# Apply all pending migrations
alembic upgrade head

# Check current revision
alembic current

# View migration history
alembic history

# Roll back one step
alembic downgrade -1
```

In Kubernetes, run migrations as an init-container or a dedicated Job before deploying the Manager.

### Per-Service Database Accounts

```sql
-- Manager service (read-write on its tables)
CREATE USER 'nest-manager-rw' IDENTIFIED BY '${MANAGER_DB_PASS}';
GRANT SELECT, INSERT, UPDATE, DELETE ON nest_db.* TO 'nest-manager-rw';

-- API Gateway (read-only for auth lookups)
CREATE USER 'nest-api-ro' IDENTIFIED BY '${API_DB_PASS}';
GRANT SELECT ON nest_db.users TO 'nest-api-ro';
GRANT SELECT ON nest_db.roles TO 'nest-api-ro';

-- Migration runner (admin, never used at runtime)
CREATE USER 'nest-migrate' IDENTIFIED BY '${MIGRATE_DB_PASS}';
GRANT ALL ON nest_db.* TO 'nest-migrate';
```

---

## 6. TLS / CA Management

NEST includes an internal Certificate Authority for managing TLS certificates across managed resources.

### CA Hierarchy

1. **Root CA**: RSA 4096-bit, 10-year validity. Generated once during initial setup.
2. **Intermediate CA**: Signed by Root CA, 3-year validity. Used for day-to-day issuance.
3. **Service Certificates**: Signed by Intermediate CA with SANs, 1-year validity, auto-rotated.

### Auto-Rotation

The `cert_rotation` background worker monitors certificate expiration and rotates certificates 30 days before expiry. Renewed certificates are stored as Kubernetes TLS Secrets.

### Manual Certificate Operations

| Endpoint                           | Method | Purpose                    |
| ---------------------------------- | ------ | -------------------------- |
| `/api/v1/certificates`             | GET    | List all certificates      |
| `/api/v1/certificates/generate`    | POST   | Generate a new certificate |
| `/api/v1/certificates/{id}/revoke` | POST   | Revoke a certificate       |
| `/api/v1/certificates/crl`         | GET    | Download current CRL       |

---

## 7. Backup Configuration

### Supported Backends

**S3:**

```bash
BACKUP_BACKEND=s3
BACKUP_S3_BUCKET=nest-backups
BACKUP_S3_REGION=us-east-1
BACKUP_S3_ACCESS_KEY=<secret>
BACKUP_S3_SECRET_KEY=<secret>
BACKUP_S3_ENDPOINT=https://s3.amazonaws.com   # Override for MinIO/Ceph
```

**NFS:**

```bash
BACKUP_BACKEND=nfs
BACKUP_NFS_SERVER=10.0.0.50
BACKUP_NFS_PATH=/exports/nest-backups
BACKUP_NFS_MOUNT_OPTIONS=vers=4,tcp
```

**Local:**

```bash
BACKUP_BACKEND=local
BACKUP_LOCAL_PATH=/var/lib/nest/backups
```

### Scheduling

The `backup_scheduler` worker executes backups on cron-style schedules configured per backup policy. Supported types: full, incremental, differential.

### Retention Policies

| Frequency | Default Retention |
| --------- | ----------------- |
| Daily     | Keep last 7       |
| Weekly    | Keep last 4       |
| Monthly   | Keep last 12      |

Retention is configurable per resource via the backup policy API.

---

## 8. Monitoring Stack

### Prometheus

Scrapes metrics from all NEST services and managed resources.

- **Port**: 9090
- **Scrape targets**: Manager (`stats_collector`), API Gateway, K8s Controller, managed databases
- **Retention**: 15 days default

### Grafana

Pre-configured dashboards:

- Resource health overview
- Database connection pools and query performance
- Backup status and history
- Threat intelligence activity
- Scaling events timeline

**Port**: 3000 | **Default credentials**: admin / admin (change on first login)

### AlertManager

Alert routing for:

- Database server unreachable
- Certificate expiration warnings (30 days)
- Backup failures
- Scaling errors
- Critical threat intelligence matches
- Disk usage thresholds

Configure notification channels in `infrastructure/monitoring/alertmanager/alertmanager.yml`.

### Rsyslog

Centralized log aggregation from all services. Logs are structured JSON.

---

## 9. RBAC & Security

### Global Roles

| Role           | Permissions                                                                         |
| -------------- | ----------------------------------------------------------------------------------- |
| **Admin**      | Full system access: manage users, teams, resources, security rules, cloud providers |
| **Maintainer** | Read/write: create/edit resources, upload SQL files. No user management             |
| **Viewer**     | Read-only: view dashboards, resources, reports                                      |

### Team Roles

| Role       | Scope                                |
| ---------- | ------------------------------------ |
| **Owner**  | Full team control including deletion |
| **Admin**  | Manage team members and settings     |
| **Member** | Standard team-scoped resource access |
| **Viewer** | Read-only team resource access       |

### JWT Scopes

Authorization is enforced via OIDC-style scopes in JWT tokens. The API Gateway RBAC middleware checks scopes on every request -- never role names directly.

```json
{
  "sub": "<user-id>",
  "scope": "users:read resources:write teams:admin",
  "tenant": "<tenant-id>",
  "teams": ["<team-id>"],
  "roles": ["maintainer"]
}
```

Standard scopes: `users:read`, `users:write`, `users:admin`, `resources:read`, `resources:write`, `resources:delete`, `teams:read`, `teams:write`, `teams:admin`, `settings:read`, `settings:write`.

### Per-Service Database Accounts

Each service has its own database credentials scoped to only the tables and operations it needs. See section 5 for details.

---

## 10. Background Workers

The Manager service runs the following background workers:

### user_sync

Synchronizes user accounts from external identity providers and managed database servers. Detects new, modified, and removed users and updates the NEST identity table.

- **Interval**: Configurable (default: 5 minutes)
- **Config**: Identity provider URLs and credentials via environment variables

### cert_rotation

Monitors TLS certificate expiration and automatically rotates certificates 30 days before expiry. Uses the internal CA hierarchy (Root CA, Intermediate CA).

- **Interval**: Hourly
- **Actions**: Generate new certificate, update K8s TLS Secret, add old cert to CRL

### backup_scheduler

Executes scheduled backups based on configured policies. Supports full, incremental, and differential types across S3, NFS, and local backends.

- **Schedules**: Daily, weekly, monthly (configurable per resource)
- **Retention**: Configurable per backup target (see section 7)

### stats_collector

Collects resource metrics from Kubernetes and external connectors. Computes risk assessments and exports Prometheus gauges.

- **Interval**: 30 seconds
- **Metrics exported**: 10 Prometheus gauges (resource counts, health status, risk levels)
- **Risk levels**: Critical, High, Medium, Low

### Additional Workers

| Worker                | Purpose                                           | Interval     |
| --------------------- | ------------------------------------------------- | ------------ |
| `db_health_checker`   | Monitor health of registered database servers     | 30s          |
| `scaling_evaluator`   | Evaluate scaling policies against current metrics | 60s          |
| `threat_intel_poller` | Poll threat intelligence feeds for indicators     | 15 min       |
| `k8s_controller`      | Reconcile K8s resource desired state              | Event-driven |

---

## 11. Troubleshooting

### Health Endpoints

| Endpoint         | Service              | Purpose                                          |
| ---------------- | -------------------- | ------------------------------------------------ |
| `/healthz`       | All                  | Liveness probe (is the process alive?)           |
| `/readyz`        | All                  | Readiness probe (is the service ready to serve?) |
| `/metrics`       | Manager, API Gateway | Prometheus metrics (port 9090)                   |
| `/api/v1/health` | API Gateway          | API health check with version info               |

### Common Issues

**Service not starting:**

1. Verify database connectivity: check `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`
2. Check logs: `kubectl --context <ctx> logs -n nest -l app=nest-manager --tail=100`
3. Verify schema: `alembic current` (inside Manager container)
4. Check K8s events: `kubectl --context <ctx> get events -n nest --sort-by=.metadata.creationTimestamp`

**Authentication failures:**

1. Verify `JWT_SECRET_KEY` is consistent across API Gateway and Manager
2. Decode JWT at jwt.io to check `exp` and `scope` claims
3. Check RBAC middleware logs for scope mismatches

**Background worker issues:**

1. Check worker logs: filter by worker name in Manager pod logs
2. Verify external connectivity (threat feeds, cloud providers, backup targets)
3. Monitor `DB_POOL_SIZE` vs active connections for pool exhaustion

**Backup failures:**

1. Verify storage backend credentials and permissions
2. Check disk space (local backend) or bucket permissions (S3)
3. Review `backup_scheduler` logs for specific error messages

**Certificate issues:**

1. List certificates: `GET /api/v1/certificates`
2. Force rotation: revoke the expiring cert; `cert_rotation` worker will issue a new one
3. Verify CA chain: check intermediate CA is valid and not expired
4. Inspect K8s TLS Secrets: `kubectl --context <ctx> get secrets -n nest -l type=tls`

**Database connectivity (managed resources):**

1. Test direct connectivity from Manager pod to target database
2. Check NetworkPolicy for cross-namespace traffic
3. For Galera clusters: verify `wsrep_ready` on all nodes

### General Debugging

```bash
# Pod status
kubectl --context <ctx> get pods -n nest

# Recent events
kubectl --context <ctx> get events -n nest --sort-by=.metadata.creationTimestamp | tail -20

# Service logs
kubectl --context <ctx> logs -n nest deployment/nest-manager --tail=50
kubectl --context <ctx> logs -n nest deployment/nest-api --tail=50
kubectl --context <ctx> logs -n nest deployment/nest-web --tail=50

# Health check (beta -- must use internal LB)
curl -H "Host: nest.penguintech.cloud" https://dal2.penguintech.cloud/api/v1/health

# Health check (alpha)
curl https://nest.localhost.local/api/v1/health
```

### Resource Connectors

| Connector      | Capabilities                                             |
| -------------- | -------------------------------------------------------- |
| **PostgreSQL** | User sync, config management, health checks, backups     |
| **MariaDB**    | User sync, config management, Galera cluster awareness   |
| **Redis**      | Health monitoring, key-space analysis, memory management |
| **Ceph**       | Pool management, health checks, capacity monitoring      |
| **SAN**        | LUN management, zoning, capacity monitoring              |

---

**Support**: support@penguintech.io | https://status.penguintech.io
