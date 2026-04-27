# Object Storage Lifecycle & Tiering

## Overview

Nest object storage provides S3-compatible storage via Ceph RGW with automatic lifecycle management and four storage tiers. Tiering enables cost optimization by transitioning data from hot (NVMe) to cold (archival HDD) storage based on age, while maintaining transparent S3 access patterns.

---

## Storage Tiers

| Tier | StorageClass | Media | Hot/Cold | Transition After | Cost |
|---|---|---|---|---|---|
| `nvme-hot` | nvme | NVMe SSD | Hot | — (never) | Highest |
| `ssd-warm` | ssd | SATA SSD | Warm | 30 days | Medium |
| `sata-bulk` | hdd | SATA HDD | Bulk | 1 year | Low |
| `sata-cold` | cold | SATA HDD (cold) | Archive | Never | Lowest |

**Default tier:** Objects start in `nvme-hot`. Lifecycle policies automatically transition objects after specified retention periods.

---

## Lifecycle Policies

Nest uses bucket-level lifecycle rules to enforce data retention and tiering. Define policies via DataResource annotations:

**Example DataResource with lifecycle:**
```yaml
apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: ml-datasets
  namespace: myapp
  annotations:
    nest.penguintech.io/lifecycle-transition-days: "30"
    nest.penguintech.io/lifecycle-expire-days: "730"
spec:
  type: object
  size: 5Ti
  storageClass: nest-object
  reclaimPolicy: Retain
  object:
    bucket: nest-myapp-datasets
```

**Annotations:**
- `lifecycle-transition-days`: Move objects from hot → warm after N days (default: 30)
- `lifecycle-expire-days`: Delete objects after N days (default: 730, ~2 years)
- `lifecycle-archive-days`: (optional) Move objects to cold tier after N days

**Example S3 lifecycle rule (advanced):**
```json
{
  "Rules": [
    {
      "ID": "warm-after-30",
      "Status": "Enabled",
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
        "Days": 730
      }
    }
  ]
}
```

---

## Bucket Naming

**Format:** `nest-{tenant}-{resource-name}`

**Examples:**
- `nest-engineering-backups`
- `nest-analytics-datasets`
- `nest-ml-models`

**Validation:** Names must be lowercase alphanumeric + hyphens, 3–63 chars, unique globally across cluster.

---

## S3 Endpoints

**Internal (in-cluster):**
```
http://nest-rgw.rook-ceph.svc.cluster.local
Port: 80 (or 443 with TLS)
```

**External (ingress-exposed):**
```
https://object.nest.penguintech.cloud
(or custom domain via Ingress/HTTPRoute)
```

**AWS CLI configuration:**
```bash
aws configure --profile nest
# AWS Access Key ID: (RGW access key)
# AWS Secret Access Key: (RGW secret key)
# Default region: us-east-1
# Default output format: json

aws s3 ls --endpoint-url http://nest-rgw.rook-ceph.svc.cluster.local \
  --profile nest
```

---

## Quota Management

Per-bucket quotas enforce storage limits, preventing runaway growth. Quotas are configured in the DataResource spec or via RGW admin API.

**Example quota in DataResource:**
```yaml
spec:
  object:
    bucket: nest-myapp-backups
    quotaMaxSizeKb: 5242880  # 5 Ti in KB
```

**Enforce quota via RGW admin:**
```bash
radosgw-admin bucket quota set --bucket=nest-myapp-backups \
  --max-size=5368709120000  # 5 Ti in bytes
radosgw-admin bucket quota enable --bucket=nest-myapp-backups
```

**Quota exceeded behavior:** New writes to bucket return `RequestLimitExceeded` (HTTP 403). Existing objects remain readable; only new puts are rejected.

---

## Monitoring & Verification

**List buckets (internal):**
```bash
aws s3 ls --endpoint-url http://nest-rgw.rook-ceph.svc.cluster.local \
  --profile nest-admin
```

**Bucket size:**
```bash
aws s3 ls s3://nest-myapp-backups --recursive --human-readable --summarize \
  --endpoint-url http://nest-rgw.rook-ceph.svc.cluster.local
```

**Lifecycle status:**
```bash
aws s3api get-bucket-lifecycle-configuration --bucket=nest-myapp-backups \
  --endpoint-url http://nest-rgw.rook-ceph.svc.cluster.local
```

**RGW metrics (Prometheus):**
```promql
rgw_put_latency_ms
rgw_get_latency_ms
rgw_bucket_size_bytes
```

---

## Best Practices

- **Tier hot data:** Use `lifecycle-transition-days: 30` for datasets accessed only initially
- **Archive by policy:** Set `lifecycle-expire-days` to auto-delete old logs/backups
- **Monitor quota:** Alert when bucket usage exceeds 80% of quota
- **Multi-part uploads:** Break large files into 100MB parts; Nest handles resumable uploads
- **Versioning:** Enable via `aws s3api put-bucket-versioning` to track object history (costs extra storage)
