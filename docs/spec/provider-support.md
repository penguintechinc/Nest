# Nest Provider Support & Feature Matrix

Authoritative reference for Nest management modes, type availability, and feature support across provisioning strategies.

---

## 1. Management Mode Overview

| Mode | `origination` value | Who provisions | Who manages lifecycle |
|------|---------------------|----------------|-----------------------|
| **Managed** | `managed` | Nest (Rook-Ceph + operators on-cluster) | Nest controller (full lifecycle) |
| **External** | `external` | Cloud provider API (AWS, Azure, GCP) | Nest controller via cloud SDK |
| **Imported** | `imported` | Customer (pre-existing resource) | Customer (Nest registers and observes only) |

---

## 2. Type Availability by Mode

| Type | Managed | External | Imported | Notes |
|------|---------|----------|----------|-------|
| `pvc/block` | ✓ | ✓ (ebs, azure-disk, gcp-disk) | ✓ | |
| `pvc/file` | ✓ | ✗ | ✓ | No cloud-native file equivalent with full parity |
| `object` | ✓ (Ceph RGW) | ✓ (s3, azure-blob, gcs) | ✓ | |
| `nfs` | ✓ | ✗ | ✓ | |
| `iscsi` | ✓ | ✗ | ✓ | |
| `postgres` | ✓ (CNPG) | ✗ | ✓ | Use `imported` to register RDS, CloudSQL, etc. |
| `keyvalue` | ✓ (Valkey/Redis) | ✗ | ✓ | Use `imported` for ElastiCache, Azure Cache, etc. |
| `search` | ✓ (OpenSearch dedicated + SearchPool shared) | ✗ | ✓ | Use `imported` for OpenSearch Service, Elastic Cloud |
| `kafka` | ✓ | ✗ | ✓ | |
| `clickhouse` | ✓ | ✗ | ✓ | |
| `trino` | ✓ | ✗ | ✓ | |
| `iceberg` | ✓ | ✗ | ✓ | |
| `timeseries` | ✓ | ✗ | ✓ | |
| `vector` | ✓ | ✗ | ✓ | |
| `mariadb` | ✓ | ✗ | ✓ | |
| `mysql` | ✓ | ✗ | ✓ | |
| `ferretdb` | ✓ | ✗ | ✓ | |

---

## 3. Feature Availability by Mode

| Feature | Managed | External | Imported |
|---------|---------|----------|----------|
| Full lifecycle (create / delete / resize) | ✓ | ✓ (via cloud API) | ✗ — register and observe only |
| DataProtectionPolicy / VolumeSnapshots | ✓ | ✗ — use cloud-native snapshots | ✗ |
| PITR (point-in-time recovery) | ✓ | ✗ | ✗ |
| Velero backup / restore | ✓ | ✗ | ✗ |
| DarkDrive-aware scheduling | ✓ | ✗ — placement set by `spec.external.availabilityZone` | ✗ |
| CSI driver / StorageClass injection | ✓ | ✗ | ✗ |
| Eggs (resource composition) | ✓ | ✗ | ✗ |
| Tenant isolation + quota | ✓ | ✓ | ✓ |
| Audit logging | ✓ | ✓ | ✓ |
| RBAC / scope enforcement | ✓ | ✓ | ✓ |
| Cost tracking | ✓ | ✓ (via cloud cost APIs) | ✗ |
| Anomaly detection | ✓ | Partial — metrics only | ✗ |
| Health probing / introspect | ✓ | ✓ | ✓ |
| Cross-region replication | ✓ (Velero) | ✓ (cloud-native replication) | ✗ |
| Shared multi-tenant OpenSearch (SearchPool) | ✓ | ✗ | ✗ |

---

## 4. Provider-Specific Notes

### Credential Secrets

Provider credentials are read from the Kubernetes Secret named by `spec.external.credentialSecret` (in the DataResource's namespace). The controller decodes the Secret and passes its data to the provisioner; credentials never need to appear in `spec.external.extra`, which is stored in cleartext on the resource. When a key is present in both the Secret and `extra`, the Secret wins.

Expected Secret keys per provider:

| Provider | Keys |
|----------|------|
| AWS | `access_key_id`, `secret_access_key`, optional `session_token` (omit entirely to use IRSA / instance role) |
| DigitalOcean | `do_token` |
| Linode | `linode_token` |
| Vultr | `vultr_api_key` |

Non-secret configuration (region, KMS key ARN, path-style flag, etc.) stays in `spec.external.extra`.

### AWS

- **EBS** (`pvc/block`): gp3 and io2 volume types; configurable IOPS and throughput; encryption at rest via KMS.
- **S3** (`object`): versioning, server-side encryption (SSE-S3, SSE-KMS), lifecycle policies.
- Credentials supplied via `spec.external.credentialSecret` (AWS access key + secret key, or IRSA annotation).
- Full provisioner implementation — create, delete, and resize operations supported.

### Azure

- **Managed Disk** (`pvc/block`): Standard HDD, Standard SSD, Premium SSD; encryption via Azure Key Vault.
- **Blob Storage** (`object`): hot/cool/archive tiers, versioning, lifecycle management.
- Credentials via `spec.external.credentialSecret` (service principal client ID + secret, or managed identity).
- Full provisioner implementation — create, delete, and resize operations supported.

### GCP

- **Persistent Disk** (`pvc/block`): standard, SSD (pd-ssd), and balanced (pd-balanced) disk types; CMEK encryption.
- **GCS** (`object`): multi-region / dual-region / region storage classes, versioning, retention policies.
- Credentials via `spec.external.credentialSecret` (service account key JSON, or Workload Identity).
- Full provisioner implementation — create, delete, and resize operations supported.

### Vultr

- **Block storage** (`pvc/block`) and **object storage** (`object`).
- **Status: Planned** — the API accepts `provider: vultr` in the DataResource spec, but the controller provisioner is not yet implemented. Create operations will return an error until the provisioner ships.

### Cloudflare

- **R2 object storage** (`object`).
- **Status: Planned** — the API accepts `provider: cloudflare` in the DataResource spec, but the controller provisioner is not yet implemented. Create operations will return an error until the provisioner ships.

---

## 5. Limitations Summary

- **Data protection is not available in external or imported mode.** Snapshots, PITR, and Velero backup/restore require managed mode. For external resources, use cloud-native tools (AWS Backup, Azure Backup, GCP snapshots).
- **Database and analytics types cannot be provisioned in external mode.** Types such as `postgres`, `keyvalue`, `search`, `kafka`, `clickhouse`, `timeseries`, `vector`, `mariadb`, `mysql`, and `ferretdb` have no external provisioner. Use `origination: imported` to register existing cloud-managed database instances.
- **DarkDrive-aware scheduling only applies to managed resources.** For external resources, node/zone placement is determined by `spec.external.availabilityZone`; Nest does not influence cloud placement beyond that field.
- **Resize operations in external mode depend on cloud API support** and may require a brief I/O pause or volume detach/reattach cycle. Behavior varies by provider and disk type.
- **Vultr and Cloudflare provisioners are planned but not yet implemented.** Setting `provider: vultr` or `provider: cloudflare` will be accepted by the API validation layer but will result in an error from the controller on any create operation.
- **Imported resources are read-only from Nest's perspective.** Nest records connection metadata, enforces tenant/RBAC policies, and exposes the resource to consumers, but never modifies or deletes the underlying resource.
- **Cost tracking is unavailable for imported resources.** Nest has no cloud API credentials scoped to external resources it did not provision and cannot attribute spend.
