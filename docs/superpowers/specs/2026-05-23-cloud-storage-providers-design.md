# Cloud Storage Provider Expansion — Design Spec

**Date:** 2026-05-23
**Branch:** v1.2.x (from v1.1.x)
**Status:** Approved

---

## Summary

Add first-class storage provider support for DigitalOcean, Vultr, and Linode (Akamai Cloud), plus a generic S3-compatible provider pathway. This is the primary feature of the v1.2.x minor release.

Each new provider supports both object storage (S3-compatible wire protocol) and block volumes (native REST API). The generic `s3-compat` provider covers any S3-compatible endpoint (Cloudflare R2, Wasabi, Backblaze B2, MinIO, etc.).

---

## Architecture

The existing `StorageProvisioner` interface and `ProvisionerFactoryMap` in `pkg/provider/storage.go` are the sole extension points. No changes are needed to the reconciler loop, CRD webhooks, or status handling. All new providers plug in via:

1. New `pkg/provider/<provider>.go` file implementing `StorageProvisioner`
2. New type constants in `apis/v1/storage_types.go`
3. Registration in `ProvisionerFactoryMap`
4. Switch cases in `controllers/external_reconciler.go`

```
DataResource (origination: external)
  └── external_reconciler.go
        └── providerForType(type, provider)
              └── ProvisionerFactoryMap["digitalocean" | "vultr" | "linode" | "s3-compat"]
                    └── StorageProvisioner
                          ├── ProvisionObjectBucket  → S3 wire protocol
                          └── ProvisionBlockVolume   → native REST API
```

---

## New Files

| File | Purpose |
|------|---------|
| `pkg/provider/do.go` | DigitalOcean Spaces + Volumes |
| `pkg/provider/vultr.go` | Vultr Object Storage + Block Storage |
| `pkg/provider/linode.go` | Linode Object Storage + Block Volumes |
| `pkg/provider/s3compat.go` | Generic S3-compatible object storage |

---

## Section 2 — Type Constants

Add to `apis/v1/storage_types.go` cloud constants block:

```go
TypeDOSpaces     = "do-spaces"      // DigitalOcean Spaces (object)
TypeDOVolume     = "do-volume"      // DigitalOcean Volumes (block)
TypeVultrObject  = "vultr-object"   // Vultr Object Storage (object)
TypeVultrBlock   = "vultr-block"    // Vultr Block Storage (block)
TypeLinodeObject = "linode-object"  // Linode Object Storage (object)
TypeLinodeBlock  = "linode-block"   // Linode Block Volumes (block)
TypeS3Compat     = "s3-compat"      // Generic S3-compatible object store
```

All seven are added to `CloudStorageTypes` so `IsCloudStorageType()` covers them automatically.

**No CRD schema changes required.** `ExternalSpec` already provides all needed fields:
- `Provider` — `"digitalocean"`, `"vultr"`, `"linode"`, or `"s3-compat"`
- `Region` — provider region slug
- `CredentialSecret` — K8s Secret name
- `Endpoint` — required for `s3-compat`; optional override for others
- `ObjectBucket` / `BlockVolume` — existing sub-specs used as-is
- `Extra` — escape hatch for provider-specific knobs (e.g. DO project ID)

---

## Section 3 — DigitalOcean Provider (`pkg/provider/do.go`)

### Spaces (Object Storage)

DO Spaces is S3-compatible. `ProvisionObjectBucket` and `DeprovisionObjectBucket` use the S3 XML wire protocol against `https://<region>.digitaloceanspaces.com`.

- Credentials from K8s Secret: `access_key` + `secret_key`
- Endpoint returned: `s3://bucket-name.region.digitaloceanspaces.com`
- Supports `ObjectBucketSpec.Versioning`, `LifecycleDays`, `PublicAccessBlock`

### Volumes (Block Storage)

Uses `https://api.digitalocean.com/v2/volumes` REST API with Bearer token.

- Credentials from K8s Secret: `do_token` key
- `SizeGB` → `size_gigabytes`
- `AvailabilityZone` → `region` (DO uses region slugs, not AZs)
- `VolumeType` defaults to `"ssd"`
- Endpoint returned: `do-volume://<volume-id>`
- `GetBlockVolumeStatus` maps DO state `"available"` → `PhaseReady`
- `DeprovisionBlockVolume` calls `DELETE /v2/volumes/<id>`

### Credential Secret Structure

```yaml
access_key: <spaces-access-key>
secret_key: <spaces-secret-key>
do_token:   <api-token>          # for Volumes API
```

All three keys can live in one secret.

---

## Section 4 — Generic S3-Compatible Provider (`pkg/provider/s3compat.go`)

Handles object buckets only against any S3-wire-protocol endpoint.

- `ExternalSpec.Endpoint` is **required** (e.g. `https://ewr1.vultrobjects.com`)
- Credentials from K8s Secret: `access_key` + `secret_key`
- `ExternalSpec.Extra["path_style"] = "true"` enables path-style URLs (MinIO, some self-hosted)
- `ProvisionObjectBucket`: `PUT /<bucket>` with S3 XML CreateBucketConfiguration
- `DeprovisionObjectBucket`: `DELETE /<bucket>`; returns clear error if bucket is non-empty rather than silently failing (surfaces as `PhaseFailed` in DataResource status)
- Block volume methods return `fmt.Errorf("s3-compat provider does not support block volumes")`

### Example providers covered

| Provider | Endpoint pattern |
|----------|-----------------|
| Cloudflare R2 | `https://<account-id>.r2.cloudflarestorage.com` |
| Wasabi | `https://s3.wasabisys.com` |
| Backblaze B2 | `https://s3.us-west-004.backblazeb2.com` |
| MinIO | `https://minio.internal` + `path_style=true` |

---

## Section 5 — Vultr Provider (`pkg/provider/vultr.go`)

### Object Storage

S3-compatible. Uses `ExternalSpec.Region` to construct endpoint `https://<region>.vultrobjects.com`.

- Credentials from K8s Secret: `access_key` + `secret_key`
- Same S3 XML wire protocol as DO Spaces

### Block Storage

Uses `https://api.vultr.com/v2/blocks` REST API with Bearer token.

- Credentials from K8s Secret: `vultr_api_key`
- `SizeGB` → `size_gb`; minimum 10 GB
- `Region` required (e.g. `"ewr"`, `"lax"`)
- `VolumeType` defaults to `"ssd_optimized"` (options: `ssd_optimized`, `high_perf`)
- Endpoint returned: `vultr-block://<block-id>`
- `GetBlockVolumeStatus` maps Vultr state `"active"` → `PhaseReady`
- `DeprovisionBlockVolume` calls `DELETE /v2/blocks/<id>`

### Credential Secret Structure

```yaml
access_key:    <object-storage-access-key>
secret_key:    <object-storage-secret-key>
vultr_api_key: <api-key>
```

---

## Section 6 — Linode Provider (`pkg/provider/linode.go`)

### Object Storage

S3-compatible. Endpoint: `https://<region>.linodeobjects.com`.

- Credentials from K8s Secret: `access_key` + `secret_key`
- Same S3 XML wire protocol

### Block Volumes

Uses `https://api.linode.com/v4/volumes` REST API with Bearer token.

- Credentials from K8s Secret: `linode_token`
- `SizeGB` → `size`; minimum 20 GB
- `Region` required (e.g. `"us-east"`, `"eu-west"`)
- `Label` defaults to `dr.Name`
- Endpoint returned: `linode-volume://<volume-id>`
- `GetBlockVolumeStatus` maps Linode state `"active"` → `PhaseReady`
- `DeprovisionBlockVolume` calls `DELETE /v4/volumes/<id>` (Linode requires volume to be detached first; provisioner checks and errors clearly if attached)

### Credential Secret Structure

```yaml
access_key:   <object-storage-access-key>
secret_key:   <object-storage-secret-key>
linode_token: <personal-access-token>
```

---

## Section 7 — Controller Changes

### `pkg/provider/storage.go` — `ProvisionerFactoryMap`

```go
"digitalocean": func() StorageProvisioner { return NewDOStorageProvisioner() },
"vultr":        func() StorageProvisioner { return NewVultrStorageProvisioner() },
"linode":       func() StorageProvisioner { return NewLinodeStorageProvisioner() },
"s3-compat":    func() StorageProvisioner { return NewS3CompatProvisioner() },
```

### `controllers/external_reconciler.go` — `providerForType()`

Add to the provider name switch:

```go
case "digitalocean":
    return kprovider.NewDOStorageProvisioner()
case "vultr":
    return kprovider.NewVultrStorageProvisioner()
case "linode":
    return kprovider.NewLinodeStorageProvisioner()
case "s3-compat":
    return kprovider.NewS3CompatProvisioner()
```

### `controllers/external_reconciler.go` — `reconcileExternalStorage()`

```go
case nestv1.TypeDOVolume, nestv1.TypeVultrBlock, nestv1.TypeLinodeBlock:
    return r.reconcileExternalBlock(ctx, dr, prov, cfg)
case nestv1.TypeDOSpaces, nestv1.TypeVultrObject, nestv1.TypeLinodeObject, nestv1.TypeS3Compat:
    return r.reconcileExternalBucket(ctx, dr, prov, cfg)
```

### `controllers/external_reconciler.go` — `reconcileExternalDelete()`

Mirror the same type additions in the delete switch.

---

## Section 8 — Testing

Each new provider file gets a corresponding `_test.go` with:

- `TestProvisionObjectBucket` — mock HTTP server returning valid S3 XML responses
- `TestDeprovisionObjectBucket` — including non-empty bucket error case
- `TestProvisionBlockVolume` — mock REST API returning valid JSON
- `TestDeprovisionBlockVolume` — including attached-volume error case for Linode
- `TestGetBlockVolumeStatus` — state mapping coverage

`controllers/object_test.go` extended with external origination cases for each new provider.

---

## Delivery Checklist

- [ ] `apis/v1/storage_types.go` — 7 new type constants + `CloudStorageTypes` entries
- [ ] `pkg/provider/do.go` — DO Spaces + Volumes
- [ ] `pkg/provider/vultr.go` — Vultr Object + Block
- [ ] `pkg/provider/linode.go` — Linode Object + Block Volumes
- [ ] `pkg/provider/s3compat.go` — generic S3-compatible object store
- [ ] `pkg/provider/storage.go` — 4 new `ProvisionerFactoryMap` entries
- [ ] `controllers/external_reconciler.go` — `providerForType()`, `reconcileExternalStorage()`, `reconcileExternalDelete()` updated
- [ ] Unit tests for all 4 new provider files
- [ ] Controller tests extended for new types
