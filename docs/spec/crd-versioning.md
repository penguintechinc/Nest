# Nest CRD Versioning Strategy

## Overview

This document defines how Nest Custom Resource Definitions (CRDs) are versioned and evolved over time while maintaining backward compatibility and enabling future schema changes.

## Current State

**All Nest CRD types are at `v1` and marked as the storage version.**

All CRD root types have the `+kubebuilder:storageversion` marker:
- `DataResource`
- `DataResourceClass`
- `Tenant`
- `HardwarePool`
- `HardwareInventory`
- `SearchPool`
- `DataProtectionPolicy`
- `DarkDrive`
- `Credential`
- `DataContract`
- `Schema`
- `WebhookSubscription`
- `Operation`
- `ResourceLabel`
- `NestFederation`
- `ComplianceBundle`

Each CRD YAML has `versions[0].served: true` and `storage: true`, confirming v1 is the current stable version.

## Version Numbering Scheme

| Version | Status | Purpose |
|---------|--------|---------|
| `v1` | Stable | Current production version; all types stored in v1 schema |
| `v1alpha1` | Reserved | Placeholder for future conversion paths or deprecation strategies |
| `v2` (future) | Next major | Breaking changes; introduced when v1 schema changes are incompatible |

## Adding Fields (No Version Bump)

When adding **new, optional fields** to existing types, no version bump is required.

**Process:**
1. Add field to the type in `apis/v1/{type}_types.go`:
   ```go
   type DataResourceSpec struct {
       // ... existing fields ...
       // NewFeature is a newly added optional field (example).
       // +kubebuilder:validation:Optional
       NewFeature *NewFeatureConfig `json:"newFeature,omitempty"`
   }
   ```

2. Update the corresponding CRD YAML in `k8s/kustomize/base/crds/{resource}.yaml` to include the new field schema in `versions[0].schema.openAPIV3Schema`.

3. Regenerate CRDs with `make manifests` (or equivalent kubebuilder command).

4. Existing stored objects are not affected; the new field is simply added to the schema and defaults to `null` when omitted.

## Renaming Fields (Requires Conversion Webhook)

When renaming or restructuring fields, a conversion webhook is required to maintain backward compatibility.

**Process:**
1. **Introduce v2 alongside v1** in the CRD YAML:
   ```yaml
   versions:
   - name: v1
     served: true
     storage: false  # Shift storage to v2
   - name: v2
     served: true
     storage: true  # v2 becomes the new storage version
     schema: ...
   ```

2. **Implement conversion logic** in `services/k8s-controller/webhook/conversion.go`:
   - Implement `ConvertFrom()` and `ConvertTo()` methods for each v2 type to convert between v1 and v2 schemas.
   - Conversion webhook routes between versions transparently.

3. **Deployment sequence:**
   - Deploy webhook (or update existing webhook if already present) with conversion logic.
   - Deploy CRD update to add v2 and set `storage: false` for v1.
   - K8s automatically stores new objects in v2; existing v1 objects converted on read/write.
   - Optionally migrate all v1 objects to v2 with `kubectl convert` or a migration job.

## Breaking Changes (Requires Major Version Bump)

When the schema changes incompatibly (e.g., renaming, removing, or retyping fields), introduce a new major version.

**Breaking change examples:**
- Renaming a field: `name` → `resourceName`
- Changing type: `spec.replicas` (int) → `spec.replicaConfig` (object)
- Removing a field entirely
- Making a required field optional or vice versa

**Process:**
1. Create `apis/v2/` directory with new type definitions.
2. Add `apis/v2/groupversion_info.go` to register the new group version.
3. Update CRD YAML to serve both v1 (served: false, storage: false) and v2 (served: true, storage: true).
4. Implement conversion webhook in `services/k8s-controller/webhook/conversion.go`.
5. Deploy and migrate at operator's discretion (can leave v1 served indefinitely if backward compatibility needed).

## Deprecation Path

To deprecate an old version while maintaining compatibility:

1. **Phase 1:** v1 served (true), storage (true) – users can still write new v1 objects.
2. **Phase 2:** v1 served (true), storage (false) – v1 still readable but new objects stored as v2.
3. **Phase 3:** v1 served (false) – v1 objects no longer accessible via API (admin migration window required).

Each phase should last at least one minor release (e.g., v1.1 → v1.2 → v1.3) to give users time to migrate.

## Validation Schema Updates

When updating CRD validation rules (OpenAPI schemas) that are **additive** (e.g., adding constraints to a new field):

1. Update the CRD YAML schema in `k8s/kustomize/base/crds/{resource}.yaml`.
2. No version bump required; validation applies immediately to all versions.

When **relaxing** validation (removing constraints), test thoroughly to ensure backward compatibility with existing stored objects.

## Testing

- **Unit tests:** Test conversion logic for round-trip conversion (v1 → v2 → v1).
- **Integration tests:** Create test objects in both versions; verify they reconcile correctly.
- **Migration tests:** Apply a CRD update with new versions; verify existing objects are accessible.

## CRD YAML Structure (Reference)

All v1 CRDs have:
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: {resources}.nest.penguintech.io
spec:
  group: nest.penguintech.io
  names:
    kind: {Kind}
    plural: {resources}
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        # ... schema definition ...
```

Conversion webhook (when introduced):
```yaml
# Add to spec.conversion section (kubebuilder generates this)
conversion:
  strategy: Webhook
  webhook:
    clientConfig:
      service:
        name: nest-webhook
        namespace: nest-system
        path: "/convert"
      port: 9443
```

## Summary

| Scenario | Action | Version Change |
|----------|--------|-----------------|
| Add optional field | Edit type, update schema | No |
| Rename field | Implement conversion webhook, add v2 | v1 → v2 (minor) |
| Breaking change | Add v2, deprecate v1 | v1 → v2 (major) |
| Relax validation | Update schema (both versions) | No |
| Remove field entirely | Implement conversion, add v2 | v1 → v2 (major) |

## See Also

- `apis/v1/` — Current stable type definitions
- `apis/v1alpha1/` — Reserved for future conversion paths
- `services/k8s-controller/webhook/conversion.go` — Conversion webhook scaffold
- `k8s/kustomize/base/crds/` — CRD YAML definitions
