# Ceph on LXD for Nest Development

Deploy a production-like Ceph storage cluster inside LXD containers for local development, testing, and validation of Nest data infrastructure. This guide walks through container setup, Rook deployment, and operational best practices.

## Overview

This solution runs a multi-node Ceph cluster on LXD (Canonical's lightweight container hypervisor), providing a complete storage platform for Nest development and testing. Unlike Docker containers, LXD supports raw block device passthrough and nested Kubernetes, enabling realistic Ceph deployments on modest hardware.

### Architecture

- **LXD containers** running Ubuntu 24.04 LTS
- **Rook Ceph operator** inside a Kubernetes cluster (MicroK8s or kubeadm)
- **Block devices** passed through from host to containers (OSDs)
- **Ceph services:** Monitor, OSD, MDS (CephFS), RGW (S3), iSCSI gateway
- **Multi-node scaling:** Start with 1 node, expand to 3+ for HA validation

### Use Cases

| Use Case | Suitability | Notes |
|----------|-------------|-------|
| Local development | Excellent | Single node, loop devices, fast iteration |
| Feature testing | Excellent | Validate DataResource CRDs, StorageClasses |
| Integration testing | Good | Multi-node testing, network failure simulation |
| Performance testing | Limited | No NUMA, shared network, not representative of bare metal |
| Production simulation | Acceptable | Demonstrates Rook deployment, operational patterns |

## Prerequisites

### Host Requirements

| Requirement | Minimum | Recommended |
|---|---|---|
| OS | Ubuntu 20.04 LTS | Ubuntu 24.04 LTS |
| LXD | 5.x | 6.x |
| RAM | 16GB | 32GB+ |
| CPU | 4 cores | 8+ cores |
| Disk | 100GB free | 200GB+ free |
| Kernel | 5.4+ | 6.x+ (for cgroup v2) |

### Software

```bash
# Install LXD (if not present)
sudo snap install lxd --classic

# Initialize LXD (accept defaults)
lxd init

# Verify installation
lxc --version
# Output: LXD X.XX

# Install kubectl (for K8s cluster)
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl && sudo mv kubectl /usr/local/bin/

# Install Helm (for Rook deployment)
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

### Storage Preparation

**Option A: Use loop devices (single node, dev/test)**

Loop devices are virtual block devices backed by files. Suitable for single-node dev/test on a laptop.

```bash
# Create loop device image files (100GB each)
for i in 1 2 3; do
  sudo fallocate -l 100G /mnt/ceph-osd-$i.img
  sudo losetup -f /mnt/ceph-osd-$i.img
done

# Verify
sudo losetup -a
# Output: /dev/loop0: [0082]:265 (/mnt/ceph-osd-1.img)
```

**Option B: Use spare SSDs/HDDs (multi-node, HA testing)**

Pass through raw block devices from the host to LXD containers. Best for realistic HA validation.

```bash
# List available disks
lsblk --nodeps -o NAME,SIZE,TYPE
# Output:
# sdb  200G disk  (not mounted)
# sdc  200G disk  (not mounted)

# Verify disks are not in use
sudo lsof /dev/sdb /dev/sdc 2>&1 || echo "Disks are free"
```

## Container Setup

### Step 1: Create LXD Profile

Create a profile that enables raw device passthrough and necessary Linux capabilities:

```bash
cat > /tmp/ceph-profile.yaml <<'EOF'
config:
  linux.kernel_modules: nf_nat,overlay,br_netfilter
  security.privileged: "true"
  security.nesting: "true"
devices:
  eth0:
    name: eth0
    nictype: bridged
    parent: lxdbr0
    type: nic
  root:
    path: /
    pool: default
    type: disk
    size: 50GB
EOF

lxc profile create ceph-cluster || true
cat /tmp/ceph-profile.yaml | lxc profile edit ceph-cluster
```

### Step 2: Launch Containers

Launch 1 or more containers. For HA testing, start with 3 nodes.

```bash
# Single node (dev/test)
lxc launch ubuntu:24.04 ceph-node-01 --profile ceph-cluster

# Multi-node (HA testing)
for i in 1 2 3; do
  lxc launch ubuntu:24.04 ceph-node-0$i --profile ceph-cluster
done

# Wait for containers to initialize (30–60 seconds)
lxc exec ceph-node-01 -- cloud-init status --wait
```

### Step 3: Attach Block Devices

Attach OSD disks to each container. Use loop devices (dev) or raw disks (HA).

**For loop devices (single node):**

```bash
# Create and attach loop devices
for i in 0 1 2; do
  # Create loop device
  LOOP_DEV=$(sudo losetup -f)
  
  # Map loop device into container
  lxc config device add ceph-node-01 osd-disk-$i disk \
    source=$LOOP_DEV \
    path=/dev/sdb
done

# Verify inside container
lxc exec ceph-node-01 -- lsblk
# Output: sdb (available for OSD)
```

**For raw disks (multi-node):**

```bash
# Attach dedicated SSD to each node
lxc config device add ceph-node-01 osd-disk-sdb disk \
  source=/dev/sdb \
  path=/dev/sdb

lxc config device add ceph-node-02 osd-disk-sdc disk \
  source=/dev/sdc \
  path=/dev/sdb

lxc config device add ceph-node-03 osd-disk-sdd disk \
  source=/dev/sdd \
  path=/dev/sdb
```

## Deploy Kubernetes Cluster

Nest requires Kubernetes to run Rook. Use either MicroK8s (simpler) or kubeadm (more control).

### Option A: MicroK8s (Recommended for Single-Node)

Deploy inside a single container. Fast and minimal setup.

```bash
# Install MicroK8s
lxc exec ceph-node-01 -- bash <<'EOF'
snap install microk8s --classic

# Enable required addons
microk8s enable dns storage
microk8s enable ingress
microk8s enable helm3

# Verify
microk8s kubectl get nodes
# Output: ceph-node-01   Ready   master   1m
EOF

# Expose kubeconfig to local machine
lxc exec ceph-node-01 -- sudo microk8s config > /tmp/ceph-kubeconfig.yaml
export KUBECONFIG=/tmp/ceph-kubeconfig.yaml

kubectl get nodes
# Output: ceph-node-01   Ready
```

### Option B: kubeadm (Recommended for Multi-Node)

Deploy across 3 containers for HA testing.

```bash
# Initialize on node 1
lxc exec ceph-node-01 -- bash <<'EOF'
# Install kubeadm, kubelet, kubectl
apt-get update && apt-get install -y kubeadm kubelet kubectl

# Initialize control plane
kubeadm init --pod-network-cidr=10.244.0.0/16

# Copy kubeconfig
mkdir -p $HOME/.kube
cp /etc/kubernetes/admin.conf $HOME/.kube/config
chown $(id -u):$(id -g) $HOME/.kube/config

# Install CNI (Flannel)
kubectl apply -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml

# Get join command
kubeadm token create --print-join-command > /tmp/join-command.sh
EOF

# Join nodes 2 and 3
for i in 2 3; do
  lxc exec ceph-node-0$i -- bash <<'EOF'
apt-get update && apt-get install -y kubeadm kubelet kubectl
bash /tmp/join-command.sh
EOF
done

# Verify cluster
lxc exec ceph-node-01 -- kubectl get nodes
# Output: 3 nodes Ready
```

## Deploy Rook Ceph Operator

Install Rook inside the Kubernetes cluster to orchestrate Ceph.

### Add Rook Helm Repository

```bash
# Add Rook Helm repo
helm repo add rook-release https://charts.rook.io/release
helm repo update

# List available Rook versions
helm search repo rook-release
# Output: rook-release/rook-ceph   vX.Y.Z
```

### Install Rook Operator

```bash
# Create namespace
kubectl create namespace rook-ceph

# Install operator
helm install rook-ceph rook-release/rook-ceph \
  --namespace rook-ceph \
  --set cephClusterSpec.mon.count=3 \
  --set cephClusterSpec.mgr.count=2 \
  --wait

# Wait for operator to be ready (2–3 minutes)
kubectl -n rook-ceph rollout status deployment/rook-ceph-operator --timeout=5m
```

### Create Ceph Cluster

Install the Rook CephCluster resource to bootstrap the cluster.

```bash
cat > /tmp/cephcluster.yaml <<'EOF'
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: quay.io/ceph/ceph:v18.2.0
  mon:
    count: 3
    allowMultiplePerNode: true  # Required for LXD (single host)
  mgr:
    count: 2
    allowMultiplePerNode: true
  osd:
    count: 3
    allowMultiplePerNode: true
    resources:
      requests:
        memory: 2Gi
  mds:
    count: 1
  rgw:
    instances: 1
  dashboard:
    enabled: true
    ssl: false
  storage:
    useAllNodes: true
    useAllDevices: true
    deviceFilter: "^sd[b-d]$"  # Match /dev/sdb, /dev/sdc, /dev/sdd
  healthCheck:
    daemonHealth:
      mon:
        interval: 45s
      osd:
        interval: 60s
EOF

kubectl apply -f /tmp/cephcluster.yaml

# Wait for Ceph cluster to initialize (5–10 minutes)
kubectl -n rook-ceph get cephcluster -w
# Output: rook-ceph   HEALTH_OK when ready
```

## Verify Cluster Health

```bash
# Access Ceph status
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph status
# Output: HEALTH_OK (or HEALTH_WARN with brief warnings)

# Check cluster capacity
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph df

# List monitors, OSDs, MGRs
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph osd tree

# Real-time cluster watch
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph -w
```

## Create Storage Pools

Create Ceph pools for RBD (block), CephFS, and RGW (S3) to be used by Nest.

### RBD Pool (Block Storage)

```bash
# Create RBD pool
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- \
  ceph osd pool create rbd_pool 64 64 replicated

# Enable RBD application
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- \
  ceph osd pool application enable rbd_pool rbd

# Verify
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph osd pool ls
# Output: rbd_pool
```

### CephFS Pool (Filesystem)

CephFS requires two pools: one for metadata, one for data.

```bash
# Create metadata pool
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- \
  ceph osd pool create cephfs_metadata 64 64 replicated

# Create data pool
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- \
  ceph osd pool create cephfs_data 128 128 replicated

# Enable CephFS application
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- \
  ceph osd pool application enable cephfs_metadata cephfs

kubectl -n rook-ceph exec deploy/rook-ceph-tools -- \
  ceph osd pool application enable cephfs_data cephfs

# Create CephFS filesystem
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- \
  ceph fs new cephfs cephfs_metadata cephfs_data

# Verify
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph fs ls
# Output: cephfs
```

## Connect Nest to Ceph

### Configure Nest StorageClasses

Create StorageClasses that Nest applications use to provision PVCs backed by Ceph:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: nest-block
provisioner: rook-ceph.rbd.csi.ceph.com
parameters:
  clusterID: rook-ceph
  pool: rbd_pool
  fstype: ext4
allowVolumeExpansion: true
reclaimPolicy: Delete

---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: nest-filesystem
provisioner: rook-ceph.cephfs.csi.ceph.com
parameters:
  clusterID: rook-ceph
  fsName: cephfs
allowVolumeExpansion: true
reclaimPolicy: Delete
```

Apply:

```bash
kubectl apply -f storageclasses.yaml

# Verify
kubectl get storageclasses
# Output: nest-block, nest-filesystem
```

### Test StorageClass with PVC

Create a test PVC to validate the StorageClass:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-pvc
spec:
  storageClassName: nest-block
  accessModes: ["ReadWriteOnce"]
  resources:
    requests:
      storage: 10Gi

---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
spec:
  containers:
  - name: busybox
    image: busybox:latest
    command: ["sleep", "3600"]
    volumeMounts:
    - name: test-vol
      mountPath: /data
  volumes:
  - name: test-vol
    persistentVolumeClaim:
      claimName: test-pvc
```

Apply and verify:

```bash
kubectl apply -f test-pvc.yaml

# Wait for PVC to bind
kubectl get pvc test-pvc --watch
# Output: test-pvc   Bound   pv-xxx   10Gi

# Verify pod mounted the volume
kubectl exec test-pod -- df /data
# Output: /dev/rbd0   10G   ...   /data
```

## Access Ceph Dashboard

The Rook operator deploys a Ceph dashboard for monitoring. Access it locally:

```bash
# Port-forward the dashboard
kubectl -n rook-ceph port-forward service/rook-ceph-mgr-dashboard 8443:8443 &

# Get dashboard password
kubectl -n rook-ceph get secret rook-ceph-dashboard-password -o jsonpath='{.data.password}' | base64 -d
# Output: <password>

# Open in browser
# https://localhost:8443
# Login: admin / <password>
```

## Scaling & Operations

### Add Nodes to Cluster

To expand from 1 to 3 nodes (multi-node testing):

```bash
# Launch additional containers
lxc launch ubuntu:24.04 ceph-node-02 --profile ceph-cluster
lxc launch ubuntu:24.04 ceph-node-03 --profile ceph-cluster

# Attach block devices to new nodes
lxc config device add ceph-node-02 osd-disk-sdb disk \
  source=/dev/sdc \
  path=/dev/sdb

# Join to Kubernetes (if using kubeadm)
lxc exec ceph-node-02 -- bash /tmp/join-command.sh

# Rook automatically discovers new nodes and adds them to Ceph
kubectl -n rook-ceph get cephcluster -o yaml | grep "osd.count"
```

### Add Storage Capacity

```bash
# Add new disk to node
lxc config device add ceph-node-01 osd-disk-sdd disk \
  source=/dev/sdd \
  path=/dev/sdd

# Rook detects and provisions new OSD automatically
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph osd tree
# New OSD appears in tree
```

### Monitor Cluster Health

```bash
# Continuous monitoring
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph -w

# Check pool status
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph df

# Check OSD utilization
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph osd df

# Detailed health report
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph health detail
```

## Cleanup

### Remove Ceph Cluster (Keep Containers)

```bash
# Delete Ceph cluster (data preserved, containers remain)
kubectl delete -f /tmp/cephcluster.yaml

# Uninstall Rook operator
helm uninstall rook-ceph -n rook-ceph
```

### Remove Everything (Full Cleanup)

```bash
# Delete all Kubernetes deployments
kubectl delete namespace rook-ceph

# Stop and remove LXD containers
for i in 1 2 3; do
  lxc delete ceph-node-0$i --force
done

# Remove loop devices (if used)
sudo losetup -d /dev/loop0 /dev/loop1 /dev/loop2
```

## Troubleshooting

### OSD Not Starting

**Symptom:** OSD pods remain Pending or CrashLoopBackOff.

**Diagnosis:**
```bash
kubectl -n rook-ceph logs -l app=rook-ceph-osd --tail=50
# Look for: device already formatted, device not found, permission denied
```

**Solutions:**
- Verify block device is attached: `lxc exec ceph-node-01 -- lsblk`
- Wipe device: `lxc exec ceph-node-01 -- sudo sgdisk -Z /dev/sdb`
- Increase pod CPU/memory: `cephClusterSpec.osd.resources.requests.memory: 4Gi`

### PVC Stays Pending

**Symptom:** PVC bound but pod stuck in Pending.

**Diagnosis:**
```bash
kubectl describe pod test-pod
# Look for: "waiting for first consumer" or "no matching nodes"
```

**Solutions:**
- For RWO PVCs: pod must be scheduled; ensure node has sufficient resources
- For RWX (CephFS): MDS must be running: `kubectl -n rook-ceph get deployment rook-ceph-mds-cephfs`

### Cluster In HEALTH_WARN

**Symptom:** `ceph status` shows HEALTH_WARN.

**Diagnosis:**
```bash
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph health detail
# Example: "4 daemons have recently crashed"
```

**Solutions:**
- Increase OSD RAM if daemons OOM: `osd.resources.requests.memory: 4Gi`
- Disable recovery throttling for faster healing: `ceph osd set nodeep-scrub`
- Restart Rook operator: `kubectl rollout restart deployment/rook-ceph-operator -n rook-ceph`

### Network Connectivity Issues

**Symptom:** Containers cannot reach each other or cluster is degraded.

**Diagnosis:**
```bash
lxc exec ceph-node-01 -- ping ceph-node-02
# Should succeed if containers are on same network

kubectl -n rook-ceph get cephcluster -o yaml | grep monitoringPath
```

**Solutions:**
- Ensure all containers are on same LXD bridge: `lxc profile list` → `ceph-cluster` → `eth0.parent`
- Enable IP forwarding: `sudo sysctl -w net.ipv4.ip_forward=1`

## Performance Notes

| Metric | Bare Metal | LXD Container |
|--------|-----------|---------------|
| OSD throughput | 500+ MB/s | 100–200 MB/s (shared network) |
| RBD latency | <1ms | 2–5ms (container overhead) |
| IOPS | 10K+ | 1–2K (limited by loop devices) |
| Scalability | 100+ nodes | 3–5 nodes (CPU/RAM constraints) |

LXD is suitable for **feature validation and integration testing**, not performance benchmarking.

## Limitations

| Limitation | Workaround |
|---|---|
| No NUMA support | Not applicable; NUMA tuning is bare-metal only |
| Shared network interface | Acceptable for dev/test; separate networks for advanced scenarios |
| Loop device IOPS | Use dedicated SSDs for realistic performance testing |
| Single LXD host | Multi-host LXD clustering requires additional setup |

## Next Steps

1. Deploy Nest and DataResource controllers on the Kubernetes cluster
2. Create DataResource manifests referencing `nest-block` and `nest-filesystem` StorageClasses
3. Test DataProtectionPolicy integration with Ceph snapshots
4. Validate multi-tenant isolation and hardware pool adoption
5. Run smoke tests for backup/restore workflows

For production deployments, scale to bare-metal Ceph or managed cloud Ceph (AWS, GCP, Azure).
