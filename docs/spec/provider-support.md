# Nest Provider Support & Feature Matrix

Authoritative reference for Nest management modes, type availability, and feature support across provisioning strategies.

---

## 1. Management Mode Overview

| Mode         | `origination` value | Who provisions                          | Who manages lifecycle                                     |
| ------------ | ------------------- | --------------------------------------- | --------------------------------------------------------- |
| **Managed**  | `managed`           | Nest (Rook-Ceph + operators on-cluster) | Nest controller (full lifecycle)                          |
| **External** | `external`          | Cloud provider API (AWS, Azure, GCP)    | Nest controller via cloud SDK                             |
| **Imported** | `imported`          | Customer (pre-existing resource)        | Customer (Nest adopts read-only — endpoint + health only) |

---

## 2. Type Availability by Mode

| Type         | Managed                                      | External                      | Imported | Notes                                                |
| ------------ | -------------------------------------------- | ----------------------------- | -------- | ---------------------------------------------------- |
| `pvc/block`  | ✓                                            | ✓ (ebs, azure-disk, gcp-disk) | ✓        |                                                      |
| `pvc/file`   | ✓                                            | ✗                             | ✓        | No cloud-native file equivalent with full parity     |
| `object`     | ✓ (Ceph RGW)                                 | ✓ (s3, azure-blob, gcs)       | ✓        |                                                      |
| `nfs`        | ✓                                            | ✗                             | ✓        |                                                      |
| `iscsi`      | ✓                                            | ✗                             | ✓        |                                                      |
| `postgres`   | ✓ (CNPG)                                     | ✗                             | ✓        | Use `imported` to register RDS, CloudSQL, etc.       |
| `keyvalue`   | ✓ (Valkey/Redis)                             | ✗                             | ✓        | Use `imported` for ElastiCache, Azure Cache, etc.    |
| `search`     | ✓ (OpenSearch dedicated + SearchPool shared) | ✗                             | ✓        | Use `imported` for OpenSearch Service, Elastic Cloud |
| `kafka`      | ✓                                            | ✗                             | ✓        |                                                      |
| `clickhouse` | ✓                                            | ✗                             | ✓        |                                                      |
| `trino`      | ✓                                            | ✗                             | ✓        |                                                      |
| `iceberg`    | ✓                                            | ✗                             | ✓        |                                                      |
| `timeseries` | ✓                                            | ✗                             | ✓        |                                                      |
| `vector`     | ✓                                            | ✗                             | ✓        |                                                      |
| `mariadb`    | ✓                                            | ✗                             | ✓        |                                                      |
| `mysql`      | ✓                                            | ✗                             | ✓        |                                                      |
| `ferretdb`   | ✓                                            | ✗                             | ✓        |                                                      |

---

## 3. Feature Availability by Mode

| Feature                                     | Managed    | External                                              | Imported                                                                               |
| ------------------------------------------- | ---------- | ----------------------------------------------------- | -------------------------------------------------------------------------------------- |
| Lifecycle (create / delete / status)        | ✓          | ✓ (via cloud API)                                     | ✗ — adopt and observe only                                                             |
| DataProtectionPolicy / VolumeSnapshots      | ✓          | ✗ — use cloud-native snapshots                        | ✗                                                                                      |
| PITR (point-in-time recovery)               | ✓          | ✗                                                     | ✗                                                                                      |
| Velero backup / restore                     | ✓          | ✗                                                     | ✗                                                                                      |
| DarkDrive-aware scheduling                  | ✓          | ✗ — placement set by `spec.external.availabilityZone` | ✗                                                                                      |
| CSI driver / StorageClass injection         | ✓          | ✗                                                     | ✗                                                                                      |
| Eggs (resource composition)                 | ✓          | ✗                                                     | ✗                                                                                      |
| Tenant isolation + quota                    | ✓          | ✓                                                     | ✓                                                                                      |
| Audit logging                               | ✓          | ✓                                                     | ✓                                                                                      |
| RBAC / scope enforcement                    | ✓          | ✓                                                     | ✓                                                                                      |
| Credential rotation                         | ✓          | ✓ (via cloud API)                                     | ✗ — `managedCredentials` rejected as unimplemented                                     |
| Failover orchestration                      | ✓          | ✗                                                     | ✗ — `managedFailover` rejected as unimplemented                                        |
| Cost tracking                               | ✓          | ✓ (via cloud cost APIs)                               | ✗                                                                                      |
| Anomaly detection                           | ✓          | Partial — metrics only                                | ✗                                                                                      |
| Health probing / introspect                 | ✓          | ✓                                                     | ✓ — TCP endpoint probe, plus provider-reported health when `spec.external` is also set |
| Cross-region replication                    | ✓ (Velero) | ✓ (cloud-native replication)                          | ✗                                                                                      |
| Shared multi-tenant OpenSearch (SearchPool) | ✓          | ✗                                                     | ✗                                                                                      |

---

## 4. Provider-Specific Notes

### Credential Secrets

Provider credentials are read from the Kubernetes Secret named by `spec.external.credentialSecret` (in the DataResource's namespace). The controller decodes the Secret and passes its data to the provisioner; credentials never need to appear in `spec.external.extra`, which is stored in cleartext on the resource. When a key is present in both the Secret and `extra`, the Secret wins.

Expected Secret keys per provider:

| Provider      | Keys                                                                                                       |
| ------------- | ---------------------------------------------------------------------------------------------------------- |
| AWS           | `access_key_id`, `secret_access_key`, optional `session_token` (omit entirely to use IRSA / instance role) |
| DigitalOcean  | `do_token` (Volumes API); `access_key`, `secret_key` (Spaces)                                              |
| Linode        | `linode_token` (Volumes API); `access_key`, `secret_key` (Object Storage)                                  |
| Vultr         | `vultr_api_key` (REST API); `access_key`, `secret_key` (Object Storage)                                    |
| S3-compatible | `access_key`, `secret_key`                                                                                 |

Non-secret configuration (region, KMS key ARN, path-style flag, etc.) stays in `spec.external.extra`.

### AWS

- **EBS** (`pvc/block`): gp3 and io2 volume types; configurable IOPS and throughput; encryption at rest via KMS.
- **S3** (`object`): versioning, server-side encryption (SSE-S3, SSE-KMS), lifecycle policies.
- Credentials supplied via `spec.external.credentialSecret` (AWS access key + secret key, or IRSA annotation).
- Provisioner implements create, delete, and status query. Resize is **not implemented** — `StorageProvisioner` exposes no resize operation.

### Azure

- **Managed Disk** (`pvc/block`): Standard HDD, Standard SSD, Premium SSD; encryption via Azure Key Vault.
- **Blob Storage** (`object`): hot/cool/archive tiers, versioning, lifecycle management.
- Credentials via `spec.external.credentialSecret` (service principal client ID + secret, or managed identity).
- Provisioner implements create, delete, and status query. Resize is **not implemented** — `StorageProvisioner` exposes no resize operation.

### GCP

- **Persistent Disk** (`pvc/block`): standard, SSD (pd-ssd), and balanced (pd-balanced) disk types; CMEK encryption.
- **GCS** (`object`): multi-region / dual-region / region storage classes, versioning, retention policies.
- Credentials via `spec.external.credentialSecret` (service account key JSON, or Workload Identity).
- Provisioner implements create, delete, and status query. Resize is **not implemented** — `StorageProvisioner` exposes no resize operation.

### DigitalOcean

- **Volumes** (`do-volume`, `pvc/block`): block volumes via the DigitalOcean REST API; create, delete, and status query. **Fully functional**.
- **Spaces** (`do-spaces`, `object`): create and delete via the S3-compatible API, AWS SigV4-signed. **Implemented and unit-tested against a mock endpoint; not yet verified against a live DigitalOcean account.**
- Credentials via `spec.external.credentialSecret` — `do_token` for the Volumes API, and a **separate** `access_key`/`secret_key` pair for Spaces. DigitalOcean issues Spaces keys independently of the account API token; the two are not interchangeable, and supplying only `do_token` fails with an explicit error.
- Region is optional — the endpoint defaults to `nyc3` and the SigV4 signing region follows the endpoint. Override with `extra["signing_region"]` if they differ.

### Linode

- **Block Volumes** (`linode-block`, `pvc/block`): block volumes via the Linode v4 API; create, delete, and status query. Linode requires a volume to be detached before deletion. **Fully functional**.
- **Object Storage** (`linode-object`, `object`): create and delete via the S3-compatible API, AWS SigV4-signed. **Implemented and unit-tested against a mock endpoint; not yet verified against a live Linode account.**
- Credentials via `spec.external.credentialSecret` — `linode_token` for the Volumes API, and a **separate** `access_key`/`secret_key` pair for Object Storage. The two are not interchangeable.
- Region is optional — the endpoint defaults to `us-east-1` and the SigV4 signing region follows the endpoint. Override with `extra["signing_region"]` if they differ.

### Vultr

- **Block Storage** (`vultr-block`, `pvc/block`): create, delete, and status query via the Vultr REST API. **Fully functional**.
- **Object Storage** (`vultr-object`, `object`): create and delete via the S3-compatible API, AWS SigV4-signed. **Implemented and unit-tested against a mock endpoint; not yet verified against a live Vultr account.**
- **Managed Databases**: discovery, health, cost data, and credential rotation via the Vultr API — used by `origination: external` and by the optional cloud-level layer of `origination: imported`.
- Credentials via `spec.external.credentialSecret` — `vultr_api_key` for the REST API, and a **separate** `access_key`/`secret_key` pair for Object Storage. The two are not interchangeable.
- Region is optional — the endpoint defaults to `ewr1` and the SigV4 signing region follows the endpoint. Override with `extra["signing_region"]` if they differ.

### S3-compatible (`s3-compat`)

- **Object storage** (`object`): create and delete against any S3-compatible endpoint (MinIO, Ceph RGW, Wasabi, Backblaze B2), AWS SigV4-signed. **Implemented and unit-tested against a mock endpoint; not yet verified against a live third-party endpoint.**
- `spec.external.endpoint` is **required** — there is no default host to infer.
- Credentials via `spec.external.credentialSecret` — `access_key`/`secret_key`, required.
- Signing region defaults to `us-east-1`, the convention for S3-compatible endpoints that do not use regions. Override with `extra["signing_region"]`.

### Cloudflare

- **D1**, **R2 object storage** (`object`), and **KV**: discovery and health checks are implemented against the Cloudflare API, so these can be adopted via `origination: imported` with `spec.external` enrichment.
- **Provisioning status: Planned** — the API accepts `provider: cloudflare` in the DataResource spec, but no storage provisioner is registered. Create operations in `origination: external` will return an error until the provisioner ships.

---

## 4a. Imported Mode

Adoption is strictly read-only. The reconciler derives an endpoint, reports health, and optionally enriches status with provider metadata; it never provisions, mutates, or deletes the adopted resource.

Two additive layers:

| Layer                      | Trigger                                     | What it gives you                                                                                                                                                                                                                              |
| -------------------------- | ------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Engine-level** (always)  | `spec.import.connectionString`              | `host:port` is extracted and TCP-probed for reachability every 60s. Works for **any** engine — RDS, Aurora, Cloud SQL, self-hosted, on-prem — because it needs only a reachable endpoint                                                       |
| **Cloud-level** (optional) | `spec.external` set alongside `spec.import` | Provider API supplies engine metadata and provider-reported health for the services listed in §4. Best-effort: on failure (IAM gap, provider outage) reconciliation falls back to the endpoint probe and the resource is **not** marked failed |

- Only `host:port` is retained — the connection string's password is never written to status or logs.
- Health maps directly to phase: healthy → `Ready`, degraded → `Degraded`, anything else → `Failed`. An adopted resource is never `Provisioning`.
- Deleting an imported DataResource releases Nest's reference only; the external resource keeps running.
- `spec.import.managedCredentials` and `spec.import.managedFailover` are **rejected**: both request that Nest mutate a resource it does not own, and neither is implemented. Setting either to `true` fails the DataResource with an explicit "not yet supported" error rather than being silently ignored.
- `spec.import.tlsMode` is accepted and recorded, but is not yet consumed by the reconciler — the probe is a plain TCP reachability check.

---

## 5. Limitations Summary

- **Data protection is not available in external or imported mode.** Snapshots, PITR, and Velero backup/restore require managed mode. For external resources, use cloud-native tools (AWS Backup, Azure Backup, GCP snapshots).
- **Database and analytics types cannot be provisioned in external mode.** Types such as `postgres`, `keyvalue`, `search`, `kafka`, `clickhouse`, `timeseries`, `vector`, `mariadb`, `mysql`, and `ferretdb` have no external provisioner. Use `origination: imported` to register existing cloud-managed database instances.
- **DarkDrive-aware scheduling only applies to managed resources.** For external resources, node/zone placement is determined by `spec.external.availabilityZone`; Nest does not influence cloud placement beyond that field.
- **Resize operations in external mode depend on cloud API support** and may require a brief I/O pause or volume detach/reattach cycle. Behavior varies by provider and disk type.
- **The Cloudflare provisioner is planned but not yet implemented.** Setting `provider: cloudflare` will be accepted by the API validation layer but will result in an error from the controller on any create operation. Cloudflare D1/R2/KV can still be adopted read-only via `origination: imported`.
- **Imported resources are read-only from Nest's perspective.** Nest records connection metadata, enforces tenant/RBAC policies, and exposes the resource to consumers, but never modifies or deletes the underlying resource.
- **`managedCredentials` and `managedFailover` are not implemented.** Setting either on an imported resource fails the DataResource outright rather than being silently ignored — Nest will not rotate credentials or perform failover on a resource it does not own.
- **Cost tracking is unavailable for imported resources.** Nest has no cloud API credentials scoped to external resources it did not provision and cannot attribute spend.
