"""Import, introspect, snapshot, and restore handlers."""
import asyncio
import time
import uuid
from datetime import datetime

from quart import jsonify, request, g
from kubernetes import client, config

from middleware import get_tenant, get_claims, emit_audit, AuditEvent
from handlers.probe import extract_host_port, tcp_ping
from store import Store
from models import OperationRecord


async def snapshot_data_resource(store: Store):
    """Snapshot a DataResource (LRO stub).

    POST /api/v1/tenants/:tenantId/data-resources/:name/snapshot
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    op_id = str(uuid.uuid4())
    now = datetime.utcnow().isoformat() + "Z"

    # Create and store the operation
    op = OperationRecord(
        id=op_id,
        tenant=tenant,
        op_type="snapshot",
        resource=name,
        phase="Running",
        started_at=now,
    )
    await store.create_operation(op)

    response = jsonify(
        {
            "operationId": op_id,
            "type": "snapshot",
            "resource": name,
            "tenant": tenant,
            "status": "RUNNING",
            "startedAt": now,
        }
    )
    response.headers["Location"] = f"/api/v1/tenants/{tenant}/operations/{op_id}"
    response.status_code = 202
    return response


async def restore_data_resource(store: Store):
    """Restore a DataResource from snapshot or Velero backup.

    POST /api/v1/tenants/:tenantId/data-resources/:name/restore

    Supports two restore modes:
    - snapshot_name: restores from a VolumeSnapshot (creates new PVC)
    - backup_name: restores from a Velero Backup (creates Restore CR)
    If backup_name is absent, finds the most recent Velero backup labeled with nest.penguintech.io/policy=<name>
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        req = await request.get_json() or {}
    except Exception:
        req = {}

    backup_name = req.get("backupName", "") or req.get("backup_name", "")
    snapshot_name = req.get("snapshotName", "") or req.get("snapshot_name", "")
    mode = req.get("mode", "side-by-side")
    if not mode:
        mode = "side-by-side"

    namespace = req.get("namespace", "default")

    # Default get DataResource to determine namespace
    try:
        dr = await store.get_data_resource(tenant, name)
        namespace = dr.namespace or namespace
    except ValueError:
        pass  # Use provided namespace or default

    try:
        # Initialize K8s client
        config.load_incluster_config()
        api_client = client.ApiClient()
        custom_api = client.CustomObjectsApi(api_client)
        v1 = client.CoreV1Api(api_client)
    except Exception as e:
        return (
            jsonify(
                {
                    "code": "nest.k8s.error",
                    "message": f"Failed to initialize Kubernetes client: {str(e)}",
                    "requestId": request_id,
                }
            ),
            500,
        )

    try:
        if snapshot_name:
            # Restore from VolumeSnapshot
            pvc_name = f"{name}-restored-{int(time.time())}"
            pvc_body = {
                "apiVersion": "v1",
                "kind": "PersistentVolumeClaim",
                "metadata": {
                    "name": pvc_name,
                    "namespace": namespace,
                    "labels": {"nest.penguintech.io/restored-from": snapshot_name},
                },
                "spec": {
                    "storageClassName": "nest-block",
                    "accessModes": ["ReadWriteOnce"],
                    "resources": {"requests": {"storage": "10Gi"}},
                    "dataSource": {
                        "name": snapshot_name,
                        "kind": "VolumeSnapshot",
                        "apiGroup": "snapshot.storage.k8s.io",
                    },
                },
            }

            v1.create_namespaced_persistent_volume_claim(namespace, pvc_body)
            restore_object_name = pvc_name
            restore_kind = "PersistentVolumeClaim"
        else:
            # Restore from Velero Backup
            if not backup_name:
                # Find most recent backup with label nest.penguintech.io/policy=<name>
                try:
                    backups = custom_api.list_namespaced_custom_object(
                        group="velero.io",
                        version="v1",
                        namespace="velero",
                        plural="backups",
                        label_selector=f"nest.penguintech.io/policy={name}",
                    )
                    if backups.get("items"):
                        # Sort by creation time and get the most recent
                        items = sorted(
                            backups["items"],
                            key=lambda x: x["metadata"].get("creationTimestamp", ""),
                            reverse=True,
                        )
                        backup_name = items[0]["metadata"]["name"]
                    else:
                        return (
                            jsonify(
                                {
                                    "code": "nest.backup.not_found",
                                    "message": f"No backups found for policy {name}",
                                    "requestId": request_id,
                                }
                            ),
                            404,
                        )
                except Exception as e:
                    return (
                        jsonify(
                            {
                                "code": "nest.k8s.error",
                                "message": f"Failed to list backups: {str(e)}",
                                "requestId": request_id,
                            }
                        ),
                        500,
                    )

            restore_name = f"{name}-restore-{int(time.time())}"
            restore_body = {
                "apiVersion": "velero.io/v1",
                "kind": "Restore",
                "metadata": {
                    "name": restore_name,
                    "namespace": "velero",
                    "labels": {
                        "nest.penguintech.io/policy": name,
                        "nest.penguintech.io/initiated-by": tenant,
                    },
                },
                "spec": {
                    "backupName": backup_name,
                    "includedNamespaces": [namespace],
                    "restorePVs": True,
                    "existingResourcePolicy": "update",
                },
            }

            custom_api.create_namespaced_custom_object(
                group="velero.io",
                version="v1",
                namespace="velero",
                plural="restores",
                body=restore_body,
            )
            restore_object_name = restore_name
            restore_kind = "Restore"

        claims = get_claims()
        op_id = str(uuid.uuid4())
        now = datetime.utcnow().isoformat() + "Z"

        # Create and store the operation
        op = OperationRecord(
            id=op_id,
            tenant=tenant,
            op_type="restore",
            resource=name,
            phase="Running",
            started_at=now,
            result={
                "restoreObject": restore_object_name,
                "restoreKind": restore_kind,
                "mode": mode,
            },
        )
        await store.create_operation(op)

        asyncio.create_task(
            emit_audit(AuditEvent(
                event_type="dataresource.restore_initiated",
                tenant=tenant,
                subject=claims.sub if claims else "",
                resource="DataResource",
                resource_name=name,
                action="restore",
                outcome="success",
                request_id=request_id,
            ))
        )
        response = jsonify(
            {
                "operationId": op_id,
                "type": "restore",
                "resource": name,
                "tenant": tenant,
                "mode": mode,
                "restoreObject": restore_object_name,
                "restoreKind": restore_kind,
                "status": "RUNNING",
                "startedAt": now,
            }
        )
        response.headers["Location"] = f"/api/v1/tenants/{tenant}/operations/{op_id}"
        response.status_code = 202
        return response

    except Exception as e:
        return (
            jsonify(
                {
                    "code": "nest.restore.error",
                    "message": f"Failed to initiate restore: {str(e)}",
                    "requestId": request_id,
                }
            ),
            500,
        )


async def introspect_imported_resource(store: Store):
    """Introspect an imported resource via TCP probe.

    POST /api/v1/tenants/:tenantId/data-resources/:name/introspect
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    try:
        dr = await store.get_data_resource(tenant, name)
    except ValueError:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.not_found",
                    "message": "DataResource not found",
                    "requestId": request_id,
                }
            ),
            404,
        )

    # Extract host:port from connection string or external endpoint
    conn_str = dr.import_conn_str or dr.external_endpoint
    if not conn_str:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.invalid",
                    "message": "No connection string or endpoint configured",
                    "requestId": request_id,
                }
            ),
            400,
        )

    host_port = extract_host_port(conn_str, default_port=5432)
    if not host_port:
        return (
            jsonify(
                {
                    "code": "nest.dataresource.invalid",
                    "message": f"Failed to parse endpoint: {conn_str}",
                    "requestId": request_id,
                }
            ),
            400,
        )

    host, port = host_port
    reachable, latency_ms, message = await tcp_ping(host, port, timeout=5.0)

    return jsonify(
        {
            "resource": name,
            "reachable": reachable,
            "latencyMs": latency_ms,
            "message": message,
        }
    )


async def migrate_to_managed(store: Store):
    """Migrate resource to managed (LRO stub).

    POST /api/v1/tenants/:tenantId/data-resources/:name/migrate
    """
    tenant = get_tenant()
    name = request.view_args.get("name", "")
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    op_id = str(uuid.uuid4())
    now = datetime.utcnow().isoformat() + "Z"

    # Create and store the operation
    op = OperationRecord(
        id=op_id,
        tenant=tenant,
        op_type="migrate",
        resource=name,
        phase="Running",
        started_at=now,
    )
    await store.create_operation(op)

    response = jsonify(
        {
            "operationId": op_id,
            "type": "migrate",
            "resource": name,
            "tenant": tenant,
            "status": "RUNNING",
            "startedAt": now,
        }
    )
    response.headers["Location"] = f"/api/v1/tenants/{tenant}/operations/{op_id}"
    response.status_code = 202
    return response
