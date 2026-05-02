# Nest Storage Deployment: Rook-Ceph on Kubernetes

**Version:** 2.0.0  
**Maintained by:** Penguin Tech Inc  
**License:** Limited AGPL3

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Architecture Overview](#architecture-overview)
3. [Deployment Steps](#deployment-steps)
4. [Configuration](#configuration)
5. [Verification](#verification)
6. [Scaling](#scaling)
7. [Upgrades](#upgrades)
8. [LXD Deployment](#lxd-deployment)

## Prerequisites

### Kubernetes Requirements

- **Kubernetes Version:** 1.28+
- **Cluster Access:** Admin context configured
- **Storage:** Dedicated raw disks for OSDs (not OS disks)
- **Namespace:** `rook-ceph` (created by Rook operator)

### Hardware Requirements

**Minimum (Development/Testing):**
- 1 node with 4+ CPU cores
- 8GB RAM
- 50GB root disk + 50GB OSD disk
- 1Gbps network

**Recommended (Production):**
- 3+ nodes with 8+ CPU cores each
- 32GB RAM per node
- Dedicated SSD for OS (100GB+)
- Dedicated SSDs/NVMe for OSD data
- 10Gbps network (separate cluster network optional)

### Network

- Reliable inter-node connectivity
- NTP synchronization across all nodes
- Optional: Separate cluster network for Ceph replication traffic

## Architecture Overview

### Component Layout

```
Kubernetes Cluster
├── Namespace: rook-ceph
│   ├── Rook Operator (1 pod)
│   ├── MON Pods (3-5)
│   ├── MGR Pods (2)
│   ├── OSD Pods (1+ per node)
│   ├── MDS Pods (2+)
│   ├── RGW Pods (2+)
│   └── CSI Drivers (node + controller)
├── Namespace: nest
│   ├── Injector Webhook
│   ├── CSI Node Plugin
│   ├── StorageClasses (branded)
│   └── VolumeSnapshotClasses
└── Namespaces: <workloads>
    └── PVCs → StorageClasses → CSI Drivers → Ceph Cluster
```

### Data Flow

```
User Pod (create PVC with storageClassName=nest-block)
    ↓
Injector Webhook (rewrite nest-block → rook-ceph-block)
    ↓
CSI Controller (rook-ceph-rbd-provisioner)
    ├─→ Create RBD image in nest-rbd-pool
    ├─→ Return PV
    └─→ Bind PVC
    ↓
Pod Scheduled
    ↓
CSI Node Plugin (nest-csi node plugin)
    ├─→ Nest CSI proxy: rbd attach → Rook RBD socket
    ├─→ Kernel: rbd map /dev/rbd0
    └─→ Mount to pod
    ↓
Pod Access Volume
```

## Deployment Steps

### Step 1: Install Rook Operator

Add Rook Helm repository and install the operator:

```bash
# Add repository
helm repo add rook-release https://charts.rook.io/release
helm repo update

# Create namespace
kubectl create namespace rook-ceph

# Install Rook operator (latest stable)
helm install rook-ceph rook-release/rook-ceph \
    --namespace rook-ceph \
    --set installCRDs=true \
    --set rbac.create=true
```

**Verify:**
```bash
kubectl get pods -n rook-ceph
# Should see rook-ceph-operator running
```

### Step 2: Deploy Rook-Ceph Cluster

Create a CephCluster CR:

```bash
# Use provided CephCluster manifest
kubectl apply -f k8s/kustomize/base/ceph-cluster/cephcluster.yaml
```

**Example CephCluster CR:**
```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v19.2.0
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false  # Prefer one MON per node
  mgr:
    count: 2
  osd:
    # Auto OSD provisioning
    useAllNodes: true
    useAllDevices: true
    # Or explicitly list devices:
    # deviceFilter: ^sd[b-z]$
  cephfsMetadataPool:
    replicated:
      size: 3
  cephfsDataPool:
    replicated:
      size: 3
  rgw:
    instances: 2
  mds:
    activeCount: 2
    activeStandby: true
  healthCheck:
    daemonHealth:
      mon:
        disabled: false
        interval: 45s
      osd:
        disabled: false
        interval: 60s
  security:
    kms:
      connectionDetails:
        KMS_PROVIDER: skauswatch
        KMS_ENDPOINT: http://skauswatch:8080
```

**Wait for cluster to initialize (5-15 minutes):**
```bash
kubectl get cephcluster -n rook-ceph
# STATUS should be CREATED when ready

kubectl get pods -n rook-ceph | grep osd
# Should see OSD pods running
```

### Step 3: Create Storage Pools

Deploy Nest storage pools (RBD, CephFS):

```bash
# RBD pool
kubectl apply -f k8s/kustomize/base/nest-rbd/cephblockpool.yaml

# CephFS filesystem and pools
kubectl apply -f k8s/kustomize/base/nest-cephfs/cephfilesystem.yaml
```

**Example RBD Pool CR:**
```yaml
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: nest-rbd-pool
  namespace: rook-ceph
spec:
  failureDomain: host
  replicated:
    size: 3
    requireSafeReplicaSize: true
    replicasPerHost: 1
  pg_autoscale_mode: "on"
```

**Verify pools:**
```bash
kubectl exec -it $(kubectl get pods -n rook-ceph | grep mon | awk '{print $1;exit}') \
    -n rook-ceph -c mon -- ceph osd pool ls
```

### Step 4: Deploy StorageClasses

Create Rook-Ceph and Nest-branded StorageClasses:

```bash
kubectl apply -f k8s/kustomize/base/nest-rbd/storageclass.yaml
kubectl apply -f k8s/kustomize/base/nest-cephfs/storageclass.yaml
```

**Verify:**
```bash
kubectl get storageclass | grep nest
# Should see nest-block, nest-filesystem, nest-file
```

### Step 5: Deploy Nest CSI Driver

The Nest CSI driver proxies to Rook-Ceph socket:

```bash
helm install nest-csi k8s/helm/nest-csi \
    --namespace nest \
    --values k8s/helm/nest-csi/values.yaml
```

**Configuration:**
```yaml
env:
  CSI_ENDPOINT: "unix:///csi/csi.sock"
  LOG_LEVEL: warn

rook:
  rbdSocket: "unix:///var/lib/kubelet/plugins/rook-ceph.rbd.csi.ceph.com/csi.sock"
  cephfsSocket: "unix:///var/lib/kubelet/plugins/rook-ceph.cephfs.csi.ceph.com/csi.sock"

nodePlugin:
  enabled: true
  tolerations:
    - operator: Exists
```

**Verify:**
```bash
kubectl get pods -n nest | grep nest-csi
# Should see DaemonSet pods running on all nodes
```

### Step 6: Deploy Injector Webhook

The injector webhook rewrites branded StorageClass names at admission:

```bash
helm install nest-injector k8s/helm/nest-injector \
    --namespace nest \
    --values k8s/helm/nest-injector/values.yaml
```

**Webhook Configuration:**
```yaml
webhook:
  name: nest-injector
  rules:
    - resources: ["persistentvolumeclaims"]
      operations: ["CREATE"]
      scope: Namespaced
  admissionReviewVersions: ["v1"]
  clientConfig:
    service:
      namespace: nest
      name: nest-injector
      path: /mutate-pvc
    caBundle: <base64-encoded-CA-cert>
```

**Verify:**
```bash
kubectl get mutatingwebhookconfigurations | grep nest-injector
```

## Configuration

### Enable Rook-Ceph Modules

Enable optional Rook-Ceph modules:

```bash
# Access Ceph tools pod
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Enable dashboard
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph mgr module enable dashboard
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph dashboard create-self-signed-cert
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph dashboard set-login-credentials admin admin

# Enable Prometheus
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph mgr module enable prometheus
```

### Configure RGW (S3)

RGW is deployed by Rook; configure S3 users:

```bash
# Create S3 user
kubectl exec -it $MON_POD -n rook-ceph -c mon -- radosgw-admin user create \
    --uid=nest-s3 \
    --display-name="Nest S3 User" \
    --email=admin@nest.local

# Create subuser for Swift
kubectl exec -it $MON_POD -n rook-ceph -c mon -- radosgw-admin subuser create \
    --uid=nest-s3 \
    --subuser=nest-s3:swift \
    --access=full
```

### Configure Encryption (Skauswatch)

Encryption is configured in StorageClass parameters. Ensure Skauswatch is deployed and reachable:

```bash
# Verify Skauswatch connection
kubectl exec -it $MON_POD -n rook-ceph -c mon -- curl -v http://skauswatch:8080/health
```

## Verification

### Cluster Health

```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Check cluster status
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph health detail

# Check OSD status
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd tree

# Check pool status
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd pool ls detail
```

### StorageClass Status

```bash
# List StorageClasses
kubectl get storageclass

# Verify default (if set)
kubectl get storageclass nest-block -o yaml | grep is-default
```

### Test PVC Creation

```bash
# Create test PVC
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc
  namespace: default
spec:
  storageClassName: nest-block
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
EOF

# Verify PVC bound
kubectl get pvc test-pvc
# STATUS should be Bound

# Cleanup
kubectl delete pvc test-pvc
```

### CSI Driver Verification

```bash
# Check CSI node plugins
kubectl get daemonset -n nest nest-csi

# Check CSI controller (if running)
kubectl get deployment -n rook-ceph csi-rbdplugin-provisioner

# Verify socket connectivity
kubectl exec -it $(kubectl get pods -n nest -l app=nest-csi -o jsonpath='{.items[0].metadata.name}') \
    -n nest -- ls -la /csi/
```

## Scaling

### Add More OSDs

To add storage capacity, add raw disks and let Rook auto-provision OSDs:

```bash
# On node, ensure disk is available (not mounted)
lsblk | grep -E '^sd[b-z]'

# Rook will auto-discover and create OSDs if useAllDevices: true
# Monitor OSD creation:
kubectl get pods -n rook-ceph | grep osd
kubectl logs -n rook-ceph -l app=ceph-osd -f
```

### Scale RGW

```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Scale RGW replicas
kubectl exec -it $MON_POD -n rook-ceph -c mon -- \
    ceph orch apply rgw default --placement="count:3"
```

### Scale MDS

For CephFS horizontal scaling:

```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Scale MDS count
kubectl exec -it $MON_POD -n rook-ceph -c mon -- \
    ceph orch apply mds nest-cephfs --placement="count:4"
```

## Upgrades

### Ceph Version Upgrade

```bash
# Update CephCluster CR with new image
kubectl patch cephcluster rook-ceph -n rook-ceph --type merge \
    -p '{"spec":{"cephVersion":{"image":"quay.io/ceph/ceph:v19.3.0"}}}'

# Monitor upgrade progress
kubectl logs -n rook-ceph -l app=ceph-osd -f | grep -i "upgrade\|version"
```

### Rook Operator Upgrade

```bash
# Upgrade Rook operator Helm chart
helm upgrade rook-ceph rook-release/rook-ceph \
    --namespace rook-ceph \
    --values rook-values.yaml

# Verify operator pod running
kubectl get pod -n rook-ceph -l app=rook-ceph-operator
```

## LXD Deployment

For development and testing, deploy Ceph in LXD containers:

```bash
# Navigate to project root
cd /path/to/nest

# Deploy single-node LXD Ceph cluster
./scripts/infrastructure/deploy-ceph-lxd.sh

# Or deploy 3-node cluster
./scripts/infrastructure/deploy-ceph-lxd.sh -n 3

# Validate
./scripts/infrastructure/validate-ceph-cluster.sh
```

See [infrastructure/lxd/ceph/README.md](../../infrastructure/lxd/ceph/README.md) for detailed LXD deployment instructions.

## Monitoring

### Dashboard Access

```bash
# Port-forward to Ceph dashboard
kubectl port-forward -n rook-ceph service/rook-ceph-mgr-dashboard 8443:8443

# Access: https://localhost:8443
# Username: admin
# Password: (from configuration)
```

### Prometheus Metrics

```bash
# Port-forward to Prometheus endpoint
kubectl port-forward -n rook-ceph svc/ceph-mgr-prometheus-svc 9283:9283

# Access metrics: http://localhost:9283/metrics
```

### Real-Time Monitoring

```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Watch cluster status
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph -w
```

## Troubleshooting

For detailed troubleshooting, see [ceph-troubleshooting.md](ceph-troubleshooting.md).

### Quick Health Check

```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Full health detail
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph health detail

# OSD status
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd stat

# PG status
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph pg stat
```

---

**Last Updated:** 2025-05-01  
**Document Version:** 2.0.0  
**Maintained by:** Penguin Tech Inc
