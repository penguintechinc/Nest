
[[_TOC_]]

# Nest — Unified Storage & Data Platform

Nest is a Kubernetes-native unified storage and data platform. It provides a CSI driver backed by Rook-Ceph RBD for PVC block storage, a REST API for managing `DataResource` objects across multiple storage backends, and supporting components (controller, scheduler, node-agent DaemonSet).

**Module:** `github.com/penguintechinc/nest`
**Default namespace:** `nest`
**API base:** `http://nest-api.nest.svc:8080/api/v1`

---

## Prerequisites

| Requirement | Notes |
|---|---|
| Kubernetes 1.28+ | MicroK8s for alpha, remote cluster for beta/prod |
| Rook-Ceph | Must be deployed before Nest CSI is functional |
| kubectl | Configured with appropriate cluster context |
| Helm 3.x | Beta and production deployments |
| Go 1.24.2+ | Building from source only |

**Cluster contexts:**

| Environment | Context | Registry |
|---|---|---|
| Alpha (local) | `local-alpha` | `localhost:32000` |
| Beta | `dal2-beta` | `ghcr.io/penguintechinc/nest` |
| Production | `{repo}-prod` | `ghcr.io/penguintechinc/nest` |

### Rook-Ceph Requirement

Nest's CSI driver (`csi.nest.penguintech.io`) requires Rook-Ceph to be running in the cluster. The Ceph RBD pool and StorageClass provisioning depend on an operational Rook-Ceph cluster. Deploy Rook-Ceph before proceeding with Nest installation.

---

## Installation

### Alpha (Local / MicroK8s)

Alpha deploys via Kustomize against the `local-alpha` context.

```bash
# Enable local registry if not already enabled
microk8s enable registry

# Build and push images to local registry
docker build -t localhost:32000/nest-api:latest apps/api/
docker push localhost:32000/nest-api:latest

# Deploy
kubectl kustomize k8s/kustomize/overlays/alpha \
  | kubectl --context local-alpha apply -f -

# Verify pods are running
kubectl --context local-alpha get pods -n nest
```

### Beta

Beta deploys via Helm from CI-built images on `ghcr.io`. Do not build and push images manually for beta.

```bash
helm upgrade --install nest k8s/helm/nest \
  --kube-context dal2-beta \
  -n nest \
  --create-namespace \
  -f k8s/helm/nest/values-beta.yaml
```

### Production

```bash
helm upgrade --install nest k8s/helm/nest \
  --kube-context {repo}-prod \
  -n nest \
  --create-namespace \
  -f k8s/helm/nest/values-prod.yaml
```

### Verify Deployment

```bash
# Check all Nest components
kubectl --context <context> get pods -n nest

# Check CSI driver registration
kubectl --context <context> get csidrivers | grep nest

# Check StorageClasses
kubectl --context <context> get storageclasses

# Tail API logs
kubectl --context <context> logs -n nest -l app=nest-api --tail=50
```

---

## StorageClasses

| StorageClass | Access Mode | Use Case |
|---|---|---|
| `nest-rbd` | RWO | Primary block storage for new workloads |
| `nest-longhorn-compat` | RWO | Migration target from Longhorn |

**Creating a PVC using Nest RBD:**

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-data
  namespace: my-app
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: nest-rbd
  resources:
    requests:
      storage: 10Gi
```

---

## API Reference

All API calls require a Bearer token with a `tenant` claim. For local development, any JWT with a `tenant` field is accepted.

**Base URL (in-cluster):** `http://nest-api.nest.svc:8080`

### Health & Status

```bash
# Liveness
curl http://nest-api.nest.svc:8080/health

# Readiness
curl http://nest-api.nest.svc:8080/ready

# Prometheus metrics
curl http://nest-api.nest.svc:8080/metrics

# Available resource types
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/catalog

# API versions
curl http://nest-api.nest.svc:8080/api/v1/versions
```

### DataResource CRUD

All DataResource operations are scoped to a tenant.

```bash
# List DataResources
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/{tenantId}/data-resources

# Get a specific DataResource
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/{tenantId}/data-resources/{name}

# Create a DataResource (returns 202 + LRO operation ID)
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d @body.json \
  http://nest-api.nest.svc:8080/api/v1/tenants/{tenantId}/data-resources

# Delete a DataResource
curl -X DELETE \
  -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/{tenantId}/data-resources/{name}
```

### Long-Running Operations (LRO)

Create and delete operations are asynchronous and return HTTP 202 with an operation ID.

```bash
# Poll LRO status
curl -H "Authorization: Bearer $TOKEN" \
  http://nest-api.nest.svc:8080/api/v1/tenants/{tenantId}/operations/{opId}
```

**LRO response example:**

```json
{
  "status": "success",
  "data": {
    "operationId": "op-abc123",
    "state": "RUNNING",
    "progress": 40,
    "resourceName": "my-postgres",
    "startedAt": "2026-04-22T10:00:00Z"
  },
  "meta": {
    "version": 1,
    "timestamp": "2026-04-22T10:00:05Z"
  }
}
```

`state` values: `PENDING`, `RUNNING`, `SUCCEEDED`, `FAILED`

---

## DataResource Types

### postgres

Provisions a managed PostgreSQL instance.

```json
{
  "name": "my-postgres",
  "kind": "postgres",
  "spec": {
    "version": "16",
    "storage": "20Gi",
    "storageClass": "nest-rbd",
    "databases": ["appdb"],
    "resources": {
      "cpu": "500m",
      "memory": "512Mi"
    }
  }
}
```

### object

Provisions S3-compatible object storage (Ceph RGW backed).

```json
{
  "name": "my-bucket",
  "kind": "object",
  "spec": {
    "bucketName": "app-assets",
    "quota": "50Gi",
    "versioning": false,
    "accessPolicy": "private"
  }
}
```

### pvc/block

Provisions a raw RWO block PVC via the Nest CSI driver.

```json
{
  "name": "my-block-volume",
  "kind": "pvc/block",
  "spec": {
    "size": "10Gi",
    "storageClass": "nest-rbd",
    "accessMode": "ReadWriteOnce"
  }
}
```

### pvc/file

Provisions a shared filesystem PVC (RWX).

```json
{
  "name": "my-shared-volume",
  "kind": "pvc/file",
  "spec": {
    "size": "10Gi",
    "accessMode": "ReadWriteMany"
  }
}
```

### keyvalue

Provisions a managed Redis/Valkey key-value store.

```json
{
  "name": "my-cache",
  "kind": "keyvalue",
  "spec": {
    "engine": "valkey",
    "version": "7",
    "maxMemory": "256Mi",
    "persistence": true,
    "persistenceStorage": "2Gi",
    "storageClass": "nest-rbd"
  }
}
```

---

## Longhorn Migration

Nest includes tooling to migrate existing Longhorn PVCs to the `nest-longhorn-compat` StorageClass.

For full migration documentation see [`docs/migration/`](migration/).

**Quick reference:**

```bash
# Step 1: Preflight checks (verifies Longhorn health, volume state)
./scripts/migration/longhorn-preflight.sh <context>

# Step 2: Migrate a specific PVC
./scripts/migration/longhorn-migrate-pvc.sh <context> <namespace> <pvc-name>

# Step 3: Drain Longhorn after all PVCs are migrated
./scripts/migration/longhorn-drain.sh <context>
```

After migration, update workload PVCs to use `nest-rbd` for new volume provisioning.

---

## Observability

Nest exposes Prometheus metrics at `/metrics` on port 8080.

```bash
# Scrape metrics
curl http://nest-api.nest.svc:8080/metrics
```

Key metric prefixes:

| Prefix | Description |
|---|---|
| `nest_api_request_duration_seconds` | API request latency histogram |
| `nest_api_requests_total` | Total API requests by method/route/status |
| `nest_dataresource_operations_total` | DataResource create/delete/get counts |
| `nest_lro_duration_seconds` | Long-running operation duration |
| `nest_csi_volume_provision_total` | CSI volume provision counts |
| `nest_csi_volume_attach_duration_seconds` | Volume attach latency |

**Kubernetes ServiceMonitor** — if Prometheus Operator is deployed, add a `ServiceMonitor` pointing to `nest-api` port 8080 path `/metrics`.

---

## Environment Variables

These variables configure the `nest-api` container.

| Variable | Default | Required | Description |
|---|---|---|---|
| `PORT` | `8080` | No | HTTP listen port |
| `VERSION` | (injected at build) | No | Application version string |
| `LICENSE_KEY` | — | No | PenguinTech license key; enables enterprise features |
| `GIN_MODE` | `debug` | Yes (prod) | Set to `release` in production |
| `DATABASE_URL` | — | Yes | PostgreSQL connection string |
| `REDIS_URL` | — | Yes | Redis/Valkey URL |
| `NEST_NAMESPACE` | `nest` | No | Kubernetes namespace for Nest components |
| `KUBECONFIG` | — | No | Path to kubeconfig for out-of-cluster access; omit for in-cluster |

---

## Building from Source

All builds run inside Docker. Do not rely on host Go toolchain for production builds.

```bash
# Build all services (containerized)
make build

# Run tests
make test

# Run linters
make lint

# Build only the API image (alpha / local dev)
docker build -t localhost:32000/nest-api:latest apps/api/
docker push localhost:32000/nest-api:latest

# Build with version injection
docker build \
  --build-arg VERSION=$(cat .version) \
  -t localhost:32000/nest-api:latest \
  apps/api/
```

**Go version:** 1.24.2 minimum. All builds use `golang:1.24-bookworm`.

**Module:** `github.com/penguintechinc/nest`
