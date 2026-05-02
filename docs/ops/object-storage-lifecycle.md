# Object Storage Lifecycle & Tiering Operations

## Overview

Nest provides S3-compatible object storage via Ceph RGW (RADOS Gateway) with integrated lifecycle management, multi-tier storage, and cross-region replication capabilities. This runbook covers provisioning object buckets, configuring lifecycle policies, managing quotas, monitoring performance, and integrating with backup systems.

**Key concepts:**
- **Object DataResources** — S3-compatible buckets provisioned via Nest CSI driver
- **Storage tiers** — Hot (NVMe) → Warm (SSD) → Bulk (HDD) → Cold (Archive) with automatic lifecycle transitions
- **Lifecycle policies** — Automated tiering, expiration, and cleanup via S3 API
- **Backup integration** — Velero BackupStorageLocation (BSL) uses Nest object storage as backup target
- **Cross-region replication** — Multi-region copies for disaster recovery and data resilience

---

## Storage Tier Architecture

### Tier Definitions

| Tier | StorageClass | Media | Latency | Cost/GB | Typical Use |
|------|---|---|---|---|---|
| **nvme-hot** | nvme | NVMe SSD | <5ms | Highest ($) | Active worksets, databases, high-frequency access |
| **ssd-warm** | ssd | SATA SSD | 10-15ms | Medium ($$) | Recent backups, warm archives (30-365 days old) |
| **sata-bulk** | hdd | SATA HDD | 50-100ms | Low ($$$) | Compliance archives (1-7 years) |
| **sata-cold** | cold | SATA HDD (cold storage) | 100-500ms | Lowest ($$$$) | Long-term retention, rarely accessed |

**Default behavior:** Objects created in `nvme-hot`. Lifecycle policies automatically transition to colder tiers based on age.

### Tier Performance Characteristics

```
Throughput:   nvme-hot (10K ops/s) > ssd-warm (1K ops/s) > ssd-bulk (100 ops/s) > cold (10 ops/s)
Availability: nvme-hot (99.99%) = ssd-warm (99.99%) > bulk (99.9%) > cold (99.5%)
Replication:  nvme-hot (3x) = ssd-warm (2x) = bulk (2x) = cold (1x)
```

---

## Provisioning Object Buckets

### Step 1: Create DataResource (Type: object)

Define an object bucket via DataResource CRD:

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: backups
  namespace: myapp
  annotations:
    nest.penguintech.io/lifecycle-transition-days: "30"
    nest.penguintech.io/lifecycle-expire-days: "730"
    nest.penguintech.io/lifecycle-archive-days: "365"
spec:
  type: object
  tenant: mycompany
  class: object-standard
  size:
    storage: 5Ti
  origination: managed
  object:
    bucket: nest-mycompany-backups
    quotaMaxSizeBytes: 5368709120000    # 5 Ti in bytes
    acl: private                        # or public-read, authenticated-read
  reclaimPolicy: Retain
```

**Apply the manifest:**

```bash
kubectl apply -f backups-dataresource.yaml

# Verify creation
kubectl get dataresource backups -n myapp -o wide
```

**Expected output:**
```
NAME      TYPE     PHASE   CAPACITY   AGE
backups   object   Bound   5Ti        2m
```

### Step 2: Retrieve Bucket Credentials

Credentials are automatically provisioned in a Kubernetes Secret:

```bash
# List secrets for the DataResource
kubectl get secrets -n myapp -o wide | grep backups

# Example: backups-rgw-credentials
kubectl get secret backups-rgw-credentials -n myapp -o jsonpath='{.data}' | jq

# Extract and decode credentials
BUCKET_NAME=$(kubectl get secret backups-rgw-credentials -n myapp \
  -o jsonpath='{.data.bucket-name}' | base64 -d)
ACCESS_KEY=$(kubectl get secret backups-rgw-credentials -n myapp \
  -o jsonpath='{.data.access-key}' | base64 -d)
SECRET_KEY=$(kubectl get secret backups-rgw-credentials -n myapp \
  -o jsonpath='{.data.secret-key}' | base64 -d)

echo "Bucket: $BUCKET_NAME"
echo "Access Key: $ACCESS_KEY"
echo "Secret Key: $SECRET_KEY"
```

### Step 3: Configure S3 Client Access

#### In-Cluster Access (Pod/Container)

Inject credentials via environment variables or Secret mount:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backup-client
  namespace: myapp
spec:
  template:
    spec:
      containers:
      - name: client
        image: alpine:3.18
        env:
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: backups-rgw-credentials
              key: access-key
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: backups-rgw-credentials
              key: secret-key
        - name: S3_ENDPOINT
          value: "http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local"
        command: ["sh"]
        args:
          - -c
          - |
            apk add aws-cli
            aws s3 ls \
              --endpoint-url $S3_ENDPOINT \
              --region us-east-1
```

#### External/Developer Access

Configure AWS CLI locally:

```bash
# 1. Install AWS CLI
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
unzip awscliv2.zip
sudo ./aws/install

# 2. Configure profile
aws configure --profile nest
# AWS Access Key ID: <ACCESS_KEY>
# AWS Secret Access Key: <SECRET_KEY>
# Default region: us-east-1
# Default output format: json

# 3. List buckets
aws s3 ls --endpoint-url http://localhost:9000 --profile nest
# (Replace localhost with RGW endpoint; port depends on ingress config)

# 4. Upload file
aws s3 cp myfile.tar.gz s3://nest-mycompany-backups/ \
  --endpoint-url http://localhost:9000 \
  --profile nest
```

---

## Lifecycle Policy Configuration

### Overview of Lifecycle Rules

Lifecycle policies automate:
- **Tiering:** Move objects from hot → warm → cold based on age
- **Expiration:** Delete objects after retention period
- **Cleanup:** Remove incomplete multipart uploads

### Step 1: Define Lifecycle via DataResource Annotations

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: ml-datasets
  namespace: mlops
  annotations:
    nest.penguintech.io/lifecycle-transition-days: "30"
    nest.penguintech.io/lifecycle-archive-days: "365"
    nest.penguintech.io/lifecycle-expire-days: "2555"  # ~7 years
spec:
  type: object
  size:
    storage: 50Ti
  object:
    bucket: nest-mlops-datasets
```

**Annotation meanings:**
- `lifecycle-transition-days: "30"` — Move from `nvme-hot` to `ssd-warm` after 30 days
- `lifecycle-archive-days: "365"` — Move to `sata-bulk` after 365 days (requires transition-days < archive-days)
- `lifecycle-expire-days: "2555"` — Delete after 2555 days (~7 years)

### Step 2: Advanced Lifecycle Rules (S3 API)

For fine-grained control, apply S3 lifecycle rules directly:

```bash
# 1. Create lifecycle rule file
cat > /tmp/lifecycle.json <<'EOF'
{
  "Rules": [
    {
      "ID": "transition-warm-30",
      "Status": "Enabled",
      "Prefix": "logs/",
      "Transitions": [
        {
          "Days": 30,
          "StorageClass": "WARM"
        },
        {
          "Days": 365,
          "StorageClass": "COLD"
        }
      ],
      "Expiration": {
        "Days": 2555
      }
    },
    {
      "ID": "abort-multipart",
      "Status": "Enabled",
      "AbortIncompleteMultipartUpload": {
        "DaysAfterInitiation": 7
      }
    },
    {
      "ID": "delete-incomplete-mpu",
      "Status": "Enabled",
      "Prefix": "uploads/",
      "Filter": {
        "Prefix": "uploads/"
      },
      "NoncurrentVersionTransitions": [
        {
          "NoncurrentDays": 30,
          "StorageClass": "WARM"
        }
      ]
    }
  ]
}
EOF

# 2. Apply lifecycle rule to bucket
aws s3api put-bucket-lifecycle-configuration \
  --bucket nest-mlops-datasets \
  --lifecycle-configuration file:///tmp/lifecycle.json \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local
```

### Step 3: Verify Lifecycle Rules

```bash
# View current lifecycle policy
aws s3api get-bucket-lifecycle-configuration \
  --bucket nest-mycompany-backups \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local
```

**Expected output:**
```json
{
    "Rules": [
        {
            "ID": "transition-warm-30",
            "Status": "Enabled",
            "Prefix": "logs/",
            "Transitions": [
                {
                    "Days": 30,
                    "StorageClass": "WARM"
                },
                {
                    "Days": 365,
                    "StorageClass": "COLD"
                }
            ],
            "Expiration": {
                "Days": 2555
            }
        }
    ]
}
```

---

## Quota Management

### Step 1: Set Bucket Quota

Prevent runaway storage growth by enforcing per-bucket quotas:

```bash
# Via DataResource spec (at creation time)
cat > quota-bucket.yaml <<'EOF'
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: user-uploads
spec:
  type: object
  object:
    bucket: nest-uploads-user123
    quotaMaxSizeBytes: 1099511627776  # 1 Ti in bytes
EOF

kubectl apply -f quota-bucket.yaml
```

### Step 2: Enforce Quota via RGW Admin API

```bash
# Enable quota on existing bucket
radosgw-admin bucket quota set \
  --bucket=nest-mycompany-backups \
  --max-size=5368709120000  # 5 Ti in bytes

radosgw-admin bucket quota enable \
  --bucket=nest-mycompany-backups

# Verify quota
radosgw-admin bucket quota get --bucket=nest-mycompany-backups
```

**Expected output:**
```
"quota": {
    "enabled": true,
    "check_on_raw": false,
    "max_size": 5368709120000,
    "max_objects": -1  # Unlimited
}
```

### Step 3: Monitor Quota Usage

```bash
# Check current usage
aws s3api head-bucket \
  --bucket nest-mycompany-backups \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local

# Detailed usage via RGW admin
radosgw-admin bucket stats --bucket=nest-mycompany-backups | jq '.usage'

# Expected output
# {
#   "rgw.main": {
#     "size": 2199023255552,    # 2 Ti in bytes
#     "size_actual": 2199023255552,
#     "size_utilized": 2199023255552,
#     "num_objects": 15000
#   }
# }
```

### Step 4: Alert on Quota Approaching Limit

```bash
# Create monitoring rule (Prometheus)
cat > quota-alert.yaml <<'EOF'
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: object-storage-quota-alerts
spec:
  groups:
  - name: object-storage
    rules:
    - alert: BucketQuotaWarning
      expr: |
        (rgw_bucket_size_bytes / rgw_bucket_quota_bytes) > 0.8
      for: 5m
      annotations:
        summary: "Bucket {{ $labels.bucket }} at 80% quota"
        
    - alert: BucketQuotaCritical
      expr: |
        (rgw_bucket_size_bytes / rgw_bucket_quota_bytes) > 0.95
      for: 2m
      annotations:
        summary: "Bucket {{ $labels.bucket }} at 95% quota"
EOF

kubectl apply -f quota-alert.yaml
```

---

## Backup Integration (Velero)

### Step 1: Create BackupStorageLocation

Configure Velero to use Nest object storage for backup target:

```yaml
apiVersion: velero.io/v1
kind: BackupStorageLocation
metadata:
  name: nest-default
  namespace: velero
spec:
  provider: aws
  bucket: nest-velero-backups
  config:
    region: us-east-1
    s3Url: "http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local"
    accessKey: <RGW_ACCESS_KEY>
    secretKey: <RGW_SECRET_KEY>
    signatureVersion: s3v4
  accessMode: ReadWrite
  default: true
```

**Apply and verify:**

```bash
kubectl apply -f nest-bsl.yaml

# Verify BSL is accessible
kubectl get backupstoragelocations -n velero nest-default -o wide
# Should show: Status: Available
```

### Step 2: Create Backups to Nest

```bash
# 1. Create backup targeting Nest BSL
velero backup create myapp-backup \
  --storage-location nest-default \
  --include-namespaces myapp \
  --wait

# Monitor progress
velero backup logs myapp-backup

# Verify in object storage
aws s3 ls s3://nest-velero-backups/backups/myapp-backup/ \
  --recursive \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local
```

### Step 3: Configure Scheduled Backups

```yaml
apiVersion: velero.io/v1
kind: Schedule
metadata:
  name: myapp-daily-backup
  namespace: velero
spec:
  schedule: "0 2 * * *"  # 2am daily
  template:
    storageLocation: nest-default
    includedNamespaces:
    - myapp
    ttl: 720h  # 30 days retention
    hooks:
      resources:
      - name: pre-backup-hook
        includedNamespaces:
        - myapp
        includedResources:
        - pods
        execOnPod:
          container: app
          command: ["/app/pre-backup.sh"]
```

**Apply schedule:**

```bash
kubectl apply -f myapp-backup-schedule.yaml

# Verify schedule
kubectl get schedules -n velero myapp-daily-backup
```

---

## Cross-Region Replication

### Step 1: Configure Secondary Region

For disaster recovery, replicate backups to a secondary region:

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: backups-replica
  namespace: backup
  annotations:
    nest.penguintech.io/cross-region-enabled: "true"
    nest.penguintech.io/cross-region-destination: "us-west-2"
spec:
  type: object
  object:
    bucket: nest-backups-replica
    crossRegionCopy:
      enabled: true
      destination: "us-west-2"
      mode: "async"           # async or sync
      lagBudgetSeconds: 300   # RPO: recover within 5 minutes
```

### Step 2: Enable Replication via DataProtectionPolicy

```yaml
apiVersion: nest.penguintech.io/v1
kind: DataProtectionPolicy
metadata:
  name: backups-with-replication
  namespace: backup
spec:
  backups:
    schedule: "0 */6 * * *"    # Every 6 hours
    destination:
      kind: object
      resource: nest-velero-backups
    crossRegionCopy:
      enabled: true
      destination: "us-west-2"
      mode: async
      lagBudgetSeconds: 3600   # RPO: 1 hour
    retention:
      daily: 7
      weekly: 4
      monthly: 12
```

**Apply policy:**

```bash
kubectl apply -f backups-protection-policy.yaml

# Monitor replication status
kubectl get dataprotectionpolicies -n backup -o wide

# Check replication lag
radosgw-admin bucket sync status --bucket=nest-velero-backups
```

### Step 3: Failover to Secondary Region

In case primary region failure:

```bash
# 1. Verify secondary bucket has recent backups
aws s3 ls s3://nest-backups-replica/ \
  --recursive \
  --endpoint-url http://<secondary-rgw-endpoint> \
  --region us-west-2

# 2. Update Velero to use secondary BSL
kubectl patch backupstoragelocations nest-default -n velero \
  --type merge \
  -p '{"spec":{"config":{"s3Url":"http://<secondary-rgw-endpoint>","region":"us-west-2"}}}'

# 3. Restore from secondary
velero restore create myapp-restore \
  --from-backup myapp-backup \
  --wait

# Verify restore completion
velero restore describe myapp-restore
```

---

## Monitoring & Observability

### Step 1: Enable RGW Metrics

Ensure Rook-Ceph exports RGW metrics to Prometheus:

```bash
# Verify RGW pod has metrics enabled
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- \
  ceph dashboard get RGW_API_BASE_URL

# Check metrics endpoint
kubectl port-forward -n rook-ceph \
  service/rook-ceph-rgw-nest-rgw 9090:9090 &

curl http://localhost:9090/metrics | grep rgw_ | head -20
```

### Step 2: Create Monitoring Dashboards

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rgw-monitoring-dashboard
  namespace: monitoring
data:
  dashboard.json: |
    {
      "dashboard": {
        "title": "Nest Object Storage",
        "panels": [
          {
            "title": "Bucket Size (GB)",
            "targets": [
              {
                "expr": "rgw_bucket_size_bytes / 1e9"
              }
            ]
          },
          {
            "title": "Object Count",
            "targets": [
              {
                "expr": "rgw_bucket_objects_count"
              }
            ]
          },
          {
            "title": "PUT Latency (ms)",
            "targets": [
              {
                "expr": "rgw_put_latency_ms"
              }
            ]
          },
          {
            "title": "GET Latency (ms)",
            "targets": [
              {
                "expr": "rgw_get_latency_ms"
              }
            ]
          },
          {
            "title": "Quota Usage (%)",
            "targets": [
              {
                "expr": "(rgw_bucket_size_bytes / rgw_bucket_quota_bytes) * 100"
              }
            ]
          }
        ]
      }
    }
```

### Step 3: Key Metrics to Monitor

| Metric | Query | Alert Threshold |
|---|---|---|
| Bucket size | `rgw_bucket_size_bytes` | > 80% of quota |
| Object count | `rgw_bucket_objects_count` | Application-specific |
| PUT latency | `rgw_put_latency_ms` | > 1000ms |
| GET latency | `rgw_get_latency_ms` | > 500ms |
| 4xx errors | `rate(rgw_http_request_errors_4xx[5m])` | > 10/sec |
| 5xx errors | `rate(rgw_http_request_errors_5xx[5m])` | > 1/sec |
| Quota usage | `(rgw_bucket_size_bytes / rgw_bucket_quota_bytes) * 100` | > 80% |

---

## Best Practices

### Data Tiering Strategy

```
┌─ Hot (NVMe)  ──┬─ Active backups (0-30 days)
│                │  Database transaction logs
│                │  Frequently accessed data
│
├─ Warm (SSD)  ──┬─ Recent backups (30-365 days)
│                │  Semi-cold archives
│                │
├─ Bulk (HDD)  ──┬─ Cold archives (1-7 years)
│                │  Compliance retention
│                │
└─ Cold Archive ─┴─ Long-term retention (7+ years)
                    Rarely accessed
```

**Recommendation:** Set lifecycle transitions at:
- Hot → Warm: 30 days (cost savings: ~30%)
- Warm → Bulk: 365 days (cost savings: ~50%)
- Bulk → Cold: 2555 days (cost savings: ~70%)
- Delete: Application-specific (retention policy)

### Quota Planning

```
# Example for 5-team shared infrastructure:
# Total capacity: 100 Ti

team-analytics:   30 Ti  (30%)  — high-volume datasets
team-backups:     40 Ti  (40%)  — daily application backups
team-compliance:  15 Ti  (15%)  — regulatory retention
team-ml:          10 Ti  (10%)  — model artifacts
team-dev:         5 Ti   (5%)   — development/staging
```

**Alert levels:**
- 70% quota used → warning (review retention policy)
- 85% quota used → critical (prepare cleanup or expansion)
- 95% quota used → urgent (new writes rejected)

### Multipart Upload Handling

For large files (>100MB), use multipart uploads:

```bash
# Create 500MB file in 50MB parts
aws s3 cp large-file.tar.gz s3://nest-backups/ \
  --metadata "upload-id=123" \
  --sse AES256 \
  --storage-class STANDARD \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local

# Monitor active multipart uploads
aws s3api list-multipart-uploads --bucket nest-backups \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local

# Cleanup incomplete uploads after 7 days via lifecycle rule
# (see lifecycle.json example above)
```

### Versioning & Rollback

Enable object versioning for recovery:

```bash
# Enable versioning
aws s3api put-bucket-versioning \
  --bucket nest-backups \
  --versioning-configuration Status=Enabled \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local

# List all versions of an object
aws s3api list-object-versions \
  --bucket nest-backups \
  --prefix myfile.txt \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local

# Restore previous version
aws s3api get-object \
  --bucket nest-backups \
  --key myfile.txt \
  --version-id <VERSION_ID> \
  myfile-previous.txt \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local

# Note: Versioning increases storage usage by ~2-3x; use lifecycle to clean old versions
```

---

## Troubleshooting

### Bucket Creation Fails

```bash
# Check RGW pod status
kubectl get pod -n rook-ceph -l app=rook-ceph-rgw

# View RGW logs
kubectl logs -n rook-ceph <rgw-pod-name>

# Common errors:
# 1. "No capacity in cluster" → OSD full
#    Fix: Add storage capacity or reduce replica count
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph osd df

# 2. "Bucket already exists" → Name collision
#    Fix: Use unique bucket name (e.g., append UUID)

# 3. "Access denied" → Invalid credentials
#    Fix: Verify AccessKey/SecretKey in secret
kubectl get secret <bucket>-rgw-credentials -o yaml
```

### Lifecycle Transition Not Happening

```bash
# Check lifecycle rule is applied
aws s3api get-bucket-lifecycle-configuration --bucket nest-backups \
  --endpoint-url http://rook-ceph-rgw-nest-rgw.rook-ceph.svc.cluster.local

# RGW lifecycle processing happens in background (~24 hour cycle)
# Force immediate processing:
radosgw-admin bucket list-bucket-objects --bucket=nest-backups | grep -E '"size"|"storage_class"'

# If objects not transitioning:
# 1. Verify object age > transition days
# 2. Check RGW has permission to modify objects
# 3. Review RGW error logs for lifecycle processing failures
kubectl logs -n rook-ceph <rgw-pod-name> | grep lifecycle
```

### Quota Exceeded Errors

```bash
# Check quota status
radosgw-admin bucket quota get --bucket=nest-backups

# Check actual usage
radosgw-admin bucket stats --bucket=nest-backups | jq '.usage'

# If quota < actual usage:
# 1. Quota enforcement may be lagging; wait 5 minutes
# 2. Review for unaccounted objects (soft deletes, orphaned uploads)
# 3. Increase quota if intentional growth is expected

# Increase quota
radosgw-admin bucket quota set \
  --bucket=nest-backups \
  --max-size=10737418240000  # 10 Ti
```

### High Latency / Slow Performance

```bash
# Check Ceph cluster health
kubectl -n rook-ceph exec deploy/rook-ceph-tools -- ceph status

# Check RGW CPU/memory usage
kubectl top pod -n rook-ceph -l app=rook-ceph-rgw --containers

# Check network bandwidth
kubectl exec -n rook-ceph <rgw-pod-name> -- iftop -n

# Remediation:
# 1. Increase RGW replicas (add pods)
kubectl patch deployment rook-ceph-rgw-nest-rgw -n rook-ceph \
  -p '{"spec":{"replicas":3}}'

# 2. Increase backend Ceph pool replication (only after consultation)
# 3. Move hot data to nvme-hot tier explicitly
# 4. Cache frequently accessed objects locally
```

### Backup Restore Failing

```bash
# Check Velero restore logs
velero restore logs <restore-name>

# Common issues:
# 1. BSL inaccessible
velero backup-location get nest-default

# 2. Insufficient quota in target bucket
radosgw-admin bucket quota get --bucket=nest-velero-backups

# 3. Object storage credentials invalid
kubectl get secret -n velero aws-credentials -o yaml | \
  grep -A5 "credentials:"
```

---

## Operational Runbook Checklist

### Daily

- [ ] Monitor bucket quota usage (< 80%)
- [ ] Check RGW pod health and resource usage
- [ ] Review error logs for failed lifecycle transitions

### Weekly

- [ ] Verify backup completion (via Velero schedule)
- [ ] Test restore from backup to alternate namespace
- [ ] Review Prometheus metrics for anomalies (latency spikes, error rates)
- [ ] Check cross-region replication lag (if enabled)

### Monthly

- [ ] Review lifecycle policies for alignment with retention requirements
- [ ] Audit bucket ACLs and access permissions
- [ ] Cleanup incomplete multipart uploads
- [ ] Review cost trends and tier utilization
- [ ] Conduct disaster recovery drill (failover to secondary region)

### Quarterly

- [ ] Capacity planning: forecast growth, plan for expansion
- [ ] Review backup retention policies for compliance
- [ ] Optimize tier transitions based on access patterns
- [ ] Document lessons learned and process improvements

---

## References

- Ceph RGW Administration: https://docs.ceph.com/en/latest/radosgw/
- Rook Object Storage: https://rook.io/docs/rook/latest/Storage-Configuration/Object-Storage-RGW/
- AWS S3 Lifecycle: https://docs.aws.amazon.com/AmazonS3/latest/dev/object-lifecycle-mgmt.html
- Velero Backup & Restore: https://velero.io/docs/
- Nest DataResource API: `/Users/penguinz/code/nest/apis/v1/dataresource_types.go`
- DataProtectionPolicy API: `/Users/penguinz/code/nest/apis/v1/dataprotectionpolicy_types.go`
