# Nest Storage Troubleshooting: Rook-Ceph & CSI

**Version:** 2.0.0  
**Maintained by:** Penguin Tech Inc  
**License:** Limited AGPL3

## Table of Contents

1. [Quick Diagnostics](#quick-diagnostics)
2. [CSI Driver Issues](#csi-driver-issues)
3. [StorageClass & PVC Issues](#storageclass--pvc-issues)
4. [Snapshot Issues](#snapshot-issues)
5. [Backup Issues](#backup-issues)
6. [Webhook Issues](#webhook-issues)
7. [DarkDrive Discovery Issues](#darkdrive-discovery-issues)
8. [Pool Scheduling Issues](#pool-scheduling-issues)
9. [Ceph Cluster Issues](#ceph-cluster-issues)
10. [Performance Issues](#performance-issues)

## Quick Diagnostics

### Health Check Commands

```bash
# Ceph cluster status
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph health detail
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph -s

# Rook operator logs
kubectl logs -n rook-ceph -l app=rook-ceph-operator -f

# CSI driver logs
kubectl logs -n nest -l app=nest-csi -f

# Injector webhook logs
kubectl logs -n nest -l app=nest-injector -f
```

### PVC Diagnostics

```bash
# Check PVC status
kubectl get pvc -A

# Describe stuck PVC
kubectl describe pvc <pvc-name> -n <namespace>

# Check CSI driver errors
kubectl get event -A --sort-by='.lastTimestamp' | tail -20
```

## CSI Driver Issues

### CSI Driver Pods Not Running

**Symptoms:** CSI node plugin DaemonSet not running, pending, or crashing

**Diagnosis:**
```bash
kubectl get daemonset -n nest nest-csi
kubectl describe daemonset -n nest nest-csi
kubectl logs -n nest -l app=nest-csi --tail=50
```

**Solutions:**

**A. Missing socket paths:**
```bash
# Verify Rook-Ceph CSI sockets exist
kubectl exec -it $(kubectl get pods -n nest -l app=nest-csi -o jsonpath='{.items[0].metadata.name}') \
    -n nest -- ls -la /var/lib/kubelet/plugins/rook-ceph.rbd.csi.ceph.com/
kubectl exec -it $(kubectl get pods -n nest -l app=nest-csi -o jsonpath='{.items[0].metadata.name}') \
    -n nest -- ls -la /var/lib/kubelet/plugins/rook-ceph.cephfs.csi.ceph.com/
```

**Solution:** Ensure Rook-Ceph CSI drivers are deployed:
```bash
kubectl get pods -n rook-ceph | grep csi
# Should see csi-*-provisioner and csi-*-plugin pods
```

**B. Permission denied on socket:**
```bash
# Check socket permissions
kubectl exec -it $(kubectl get pods -n nest -l app=nest-csi -o jsonpath='{.items[0].metadata.name}') \
    -n nest -- stat /var/lib/kubelet/plugins/rook-ceph.rbd.csi.ceph.com/csi.sock
```

**Solution:** Mount with correct permissions in CSI pod (check Helm values).

**C. CSI endpoint not writable:**
```bash
kubectl logs -n nest -l app=nest-csi | grep -i "csi_endpoint\|socket"
```

**Solution:** Verify CSI_ENDPOINT env var and `/csi` volume mount.

### CSI Driver Proxy Connection Failed

**Symptoms:** PVC provisioning fails with "connection refused" or "socket not found"

**Diagnosis:**
```bash
# Check socket connectivity from Nest CSI pod
kubectl exec -it $(kubectl get pods -n nest -l app=nest-csi -o jsonpath='{.items[0].metadata.name}') \
    -n nest -- bash -c 'curl --unix-socket /var/lib/kubelet/plugins/rook-ceph.rbd.csi.ceph.com/csi.sock http://csi/'

# Check Rook-Ceph CSI pod logs
kubectl logs -n rook-ceph -l app=ceph-rbd-csi-node --tail=50
```

**Solution:** Restart Nest CSI DaemonSet to re-establish socket connection:
```bash
kubectl rollout restart daemonset -n nest nest-csi
```

## StorageClass & PVC Issues

### StorageClass Not Found or Not Registered

**Symptoms:** PVC creation fails with "storageclass not found"

**Diagnosis:**
```bash
# List available StorageClasses
kubectl get storageclass

# Check for both Rook and Nest branded classes
kubectl get storageclass | grep -E "rook-ceph-block|nest-block|rook-cephfs|nest-filesystem|nest-file"
```

**Solutions:**

**A. Missing StorageClass:**
```bash
# Deploy StorageClasses
kubectl apply -f k8s/kustomize/base/nest-rbd/storageclass.yaml
kubectl apply -f k8s/kustomize/base/nest-cephfs/storageclass.yaml
```

**B. Verify provisioner exists:**
```bash
kubectl get storageclass rook-ceph-block -o yaml | grep provisioner
# Should show: rook-ceph.rbd.csi.ceph.com
```

### PVC Stuck in Pending

**Symptoms:** PVC created but status remains `Pending`, never binds

**Diagnosis:**
```bash
kubectl describe pvc <pvc-name> -n <namespace>
# Check events section for error messages

# Check if provisioner is running
kubectl get pods -n rook-ceph | grep provisioner

# Check CSI provisioner logs
kubectl logs -n rook-ceph -l app=ceph-rbd-csi-provisioner -f
```

**Solutions:**

**A. Provisioner pod not running:**
```bash
kubectl get pods -n rook-ceph -l app=ceph-rbd-csi-provisioner
# If missing or unhealthy, restart:
kubectl rollout restart deployment -n rook-ceph ceph-rbd-csi-provisioner
```

**B. StorageClass provisioner mismatch:**
```bash
# Check StorageClass provisioner matches deployed CSI drivers
kubectl get storageclass nest-block -o yaml | grep provisioner
kubectl get pods -n rook-ceph | grep csi
```

**Solution:** Ensure StorageClass references correct provisioner for deployed CSI driver.

**C. Ceph pool doesn't exist:**
```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd pool ls
# Should include nest-rbd-pool for RBD StorageClasses
```

**Solution:** Create missing pool:
```bash
kubectl apply -f k8s/kustomize/base/nest-rbd/cephblockpool.yaml
```

### PVC Not Binding to Pod

**Symptoms:** PVC created and bound, but pod can't attach/mount

**Diagnosis:**
```bash
kubectl describe pod <pod-name> -n <namespace>
# Check Events for mount/attach failures

kubectl logs -n rook-ceph -l app=ceph-rbd-csi-node --tail=50 | grep -i error
```

**Solutions:**

**A. Node plugin not running on pod's node:**
```bash
kubectl get daemonset -n nest nest-csi
kubectl get pods -n nest -o wide | grep nest-csi
# Verify Nest CSI pod on same node as workload pod
```

**Solution:** Check node labels/taints; add tolerations if needed.

**B. RBD kernel module not loaded:**
```bash
kubectl exec -it <pod-name> -n <namespace> -- modprobe rbd
```

**Solution:** Ensure rbd kernel module available on node.

**C. Device mapping failed:**
```bash
kubectl logs -n rook-ceph -l app=ceph-rbd-csi-node | grep "rbd map"
```

**Solution:** Check Ceph pool and credential permissions.

## Snapshot Issues

### VolumeSnapshot Not Creating

**Symptoms:** VolumeSnapshot CR created but status doesn't progress

**Diagnosis:**
```bash
kubectl get volumesnapshot -A
kubectl describe volumesnapshot <snapshot-name> -n <namespace>

# Check VolumeSnapshotClass exists
kubectl get volumesnapshotclass | grep nest
```

**Solutions:**

**A. VolumeSnapshotClass missing:**
```bash
kubectl get volumesnapshotclass
# Should see nest-rbd-snapshot and nest-cephfs-snapshot

# If missing, deploy:
kubectl apply -f k8s/kustomize/base/nest-rbd/volumesnapshotclass.yaml
kubectl apply -f k8s/kustomize/base/nest-cephfs/volumesnapshotclass.yaml
```

**B. Snapshot controller not running:**
```bash
kubectl get deployment -n rook-ceph csi-rbdplugin-provisioner
kubectl logs -n rook-ceph -l app=ceph-rbd-csi-provisioner | grep -i snapshot
```

**Solution:** Deploy snapshot controller:
```bash
helm install csi-snapshotter rook-release/csi-snapshotter \
    --namespace rook-ceph \
    --values snapshotter-values.yaml
```

**C. RBD image doesn't support snapshots:**
```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Check RBD image features
kubectl exec -it $MON_POD -n rook-ceph -c mon -- rbd info nest-rbd-pool/<image-name>
# Should include: features: [..., deep-flatten, journaling, ...]
```

**Solution:** Recreate RBD image with snapshot support:
```bash
rbd create nest-rbd-pool/<image-name> --size 10G \
    --image-feature layering,deep-flatten,fast-diff,object-map,journaling
```

## Backup Issues

### Velero Backup Failing

**Symptoms:** BackupStorageLocation phase != Available, backups stuck

**Diagnosis:**
```bash
kubectl get backupstoragelocation -A
kubectl describe backupstoragelocation nest-default -n velero

# Check Velero pod logs
kubectl logs -n velero -l app=velero -f
```

**Solutions:**

**A. RGW endpoint not reachable:**
```bash
kubectl exec -it $(kubectl get pods -n rook-ceph -l app=ceph-rgw -o jsonpath='{.items[0].metadata.name}') \
    -n rook-ceph -- ceph orch ps | grep rgw
# Verify RGW pods running

# Test endpoint from Velero pod
kubectl exec -it $(kubectl get pods -n velero -l app=velero -o jsonpath='{.items[0].metadata.name}') \
    -n velero -- curl -v http://ceph-rgw.rook-ceph:8080
```

**Solution:** Fix RGW endpoint or credentials in BackupStorageLocation.

**B. S3 credentials invalid:**
```bash
kubectl get backupstoragelocation nest-default -n velero -o yaml | grep accessKey
```

**Solution:** Regenerate S3 credentials and update secret:
```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')
kubectl exec -it $MON_POD -n rook-ceph -c mon -- radosgw-admin user info --uid=nest-s3
```

**C. RGW bucket doesn't exist:**
```bash
# Create backup bucket
kubectl exec -it $(kubectl get pods -n rook-ceph -l app=ceph-rgw -o jsonpath='{.items[0].metadata.name}') \
    -n rook-ceph -- radosgw-admin bucket create --bucket=nest-backups --uid=nest-s3
```

### Backup Stuck or Incomplete

**Symptoms:** Backup job running for hours, not completing

**Diagnosis:**
```bash
kubectl get backup -A
kubectl describe backup <backup-name> -n velero

# Check backup logs
velero backup logs <backup-name>

# Check Velero pod resource usage
kubectl top pod -n velero
```

**Solutions:**

**A. Resource constraints:**
```bash
# Check node resources
kubectl top nodes

# Increase Velero resource requests/limits
kubectl patch deployment -n velero velero --type='json' \
    -p='[{"op":"replace","path":"/spec/template/spec/containers/0/resources/limits/memory","value":"2Gi"}]'
```

**B. Network throughput limited:**
```bash
# Check network metrics
kubectl exec -it $(kubectl get pods -n rook-ceph -l app=ceph-rgw -o jsonpath='{.items[0].metadata.name}') \
    -n rook-ceph -- iotop
```

**Solution:** Increase timeout or reduce backup batch size.

## Webhook Issues

### Injector Webhook Not Rewriting StorageClass

**Symptoms:** PVC created with `storageClassName: nest-block` but StorageClass name not rewritten to `rook-ceph-block`

**Diagnosis:**
```bash
# Check webhook pods
kubectl get pods -n nest -l app=nest-injector

# Check webhook logs
kubectl logs -n nest -l app=nest-injector -f

# Verify webhook configuration
kubectl get mutatingwebhookconfigurations | grep nest
kubectl describe mutatingwebhookconfigurations nest-injector
```

**Solutions:**

**A. Webhook not responding:**
```bash
# Restart injector deployment
kubectl rollout restart deployment -n nest nest-injector

# Verify pod running
kubectl get pods -n nest -l app=nest-injector
```

**B. TLS certificate expired or invalid:**
```bash
# Check webhook TLS cert
kubectl get secret -n nest nest-injector-certs -o yaml | grep -A5 tls.crt

# Regenerate certificate
kubectl delete secret -n nest nest-injector-certs
# Redeploy injector:
helm upgrade nest-injector k8s/helm/nest-injector -n nest
```

**C. Webhook rules don't match namespace:**
```bash
kubectl get mutatingwebhookconfigurations nest-injector -o yaml | grep -A10 namespaceSelector
```

**Solution:** Update webhook to include target namespace:
```bash
kubectl patch mutatingwebhookconfigurations nest-injector --type='json' \
    -p='[{"op":"replace","path":"/webhooks/0/namespaceSelector/matchLabels","value":{"webhook.nest":"enabled"}}]'
```

**D. Bypass webhook check:**
If webhook is down, verify branded StorageClasses exist as real resources:
```bash
kubectl get storageclass nest-block
# Should exist and be valid even if webhook is bypassed
```

## DarkDrive Discovery Issues

### HardwareInventory Not Created

**Symptoms:** Node-agent not publishing HardwareInventory CRs

**Diagnosis:**
```bash
kubectl get hardwareinventory -n nest
# Should see one per node

# Check node-agent pod on node
kubectl get pods -n nest -l app=node-agent -o wide

# Check node-agent logs
kubectl logs -n nest -l app=node-agent -f
```

**Solutions:**

**A. Node-agent DaemonSet not deployed:**
```bash
kubectl get daemonset -n nest node-agent
# If missing:
kubectl apply -f k8s/kustomize/base/nest-node-agent/daemonset.yaml
```

**B. Node-agent pod crashing:**
```bash
kubectl describe pod -n nest -l app=node-agent
kubectl logs -n nest -l app=node-agent --previous
```

**Solution:** Check pod logs for permission errors, missing volumes, or service account.

**C. lsblk failing in container:**
```bash
kubectl exec -it $(kubectl get pods -n nest -l app=node-agent -o jsonpath='{.items[0].metadata.name}') \
    -n nest -- lsblk
```

**Solution:** Ensure node-agent has privileged context or required capabilities.

### DarkDrive Not Detected

**Symptoms:** Disk exists but not showing in HardwareInventory

**Diagnosis:**
```bash
# Check HardwareInventory on node
kubectl get hardwareinventory <node-name> -o yaml

# Manually check disk from node
kubectl exec -it $(kubectl get pods -n nest -l app=node-agent -o jsonpath='{.items[0].metadata.name}') \
    -n nest -- lsblk --all
```

**Solutions:**

**A. Disk is mounted or in use:**
```bash
# Check if disk is mounted
kubectl exec -it ... -- mountpoint /dev/sdb

# Unmount if needed (careful!)
kubectl exec -it ... -- umount /dev/sdb
```

**B. Disk filtered by node-agent logic:**
```bash
# Check node-agent filtering rules
kubectl get daemonset -n nest node-agent -o yaml | grep -A5 "nodefilter\|devicefilter"
```

**Solution:** Update node-agent filters to include disk (e.g., `-/dev/sda` to exclude only boot disk).

## Pool Scheduling Issues

### DarkDriveCount Not Updated in Pool Status

**Symptoms:** Pool status shows DarkDriveCount: 0 even though DarkDrives available

**Diagnosis:**
```bash
# Check pool status
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd pool ls detail | grep nest-rbd-pool

# Check scheduler logs
kubectl logs -n nest -l app=scheduler -f
```

**Solutions:**

**A. Scheduler not watching HardwareInventory:**
```bash
# Verify scheduler RBAC
kubectl get clusterrolebinding | grep scheduler

# Check scheduler can read HardwareInventory
kubectl auth can-i get hardwareinventory --as=system:serviceaccount:nest:scheduler
```

**Solution:** Grant scheduler read permission on HardwareInventory CRs.

**B. Pool status update loop not running:**
```bash
# Restart scheduler
kubectl rollout restart deployment -n nest nest-scheduler
```

## Ceph Cluster Issues

### Cluster Health HEALTH_WARN

**Symptoms:** `ceph health` returns HEALTH_WARN with specific messages

**Diagnosis:**
```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph health detail
```

### OSDs Down or Slow

**Symptoms:** OSD marked down, rebalancing stalled, slow requests

**Diagnosis:**
```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Check OSD status
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd tree

# Check OSD logs
kubectl logs -n rook-ceph -l app=ceph-osd -f | head -50

# Check OSD performance
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd perf
```

**Solutions:**

**A. Restart failed OSD:**
```bash
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph orch daemon restart osd.<osd-id>
```

**B. Mark OSD out to speed recovery:**
```bash
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd out <osd-id>
# Wait for recovery
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph -w
# Mark back in
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd in <osd-id>
```

### MON Quorum Lost

**Symptoms:** Cluster unresponsive, "unable to get monitor info"

**Diagnosis:**
```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph mon stat
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph quorum_status
```

**Solution:** See [ceph-deployment.md](ceph-deployment.md) recovery procedures.

## Performance Issues

### Slow Volume Access

**Symptoms:** PVC reads/writes slow, high latency

**Diagnosis:**
```bash
MON_POD=$(kubectl get pods -n rook-ceph -l app=ceph-mon -o jsonpath='{.items[0].metadata.name}')

# Check OSD load
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph osd perf

# Check cluster recovery/rebalancing
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph pg stat

# Check network bandwidth
kubectl top nodes
```

**Solutions:**

**A. Rebalancing in progress:**
```bash
# Monitor progress
kubectl exec -it $MON_POD -n rook-ceph -c mon -- ceph pg stat
# Wait for "clean" state
```

**B. OSD under-provisioned:**
```bash
# Check OSD resource allocation
kubectl get pod -n rook-ceph -l app=ceph-osd -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[0].resources}{"\n"}{end}'

# Increase resources if needed (requires CephCluster CR update)
```

**C. Network saturation:**
```bash
# Check network utilization on nodes
kubectl exec -it $(kubectl get pods -n rook-ceph -l app=ceph-osd -o jsonpath='{.items[0].metadata.name}') \
    -n rook-ceph -- iftop
```

**Solution:** Provision additional network capacity or separate cluster network.

---

**Last Updated:** 2025-05-01  
**Document Version:** 2.0.0  
**Maintained by:** Penguin Tech Inc
