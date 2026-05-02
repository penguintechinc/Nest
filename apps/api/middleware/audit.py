"""Audit event emitter — fire-and-forget POST to the audit service."""
import asyncio
import json
import logging
import os
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime, timezone

logger = logging.getLogger(__name__)

_DEFAULT_AUDIT_URL = "http://nest-audit.nest.svc.cluster.local:8085"


@dataclass(slots=True)
class AuditEvent:
    """Represents a single audit log entry sent to the audit service."""

    event_type: str       # e.g. "dataresource.created"
    tenant: str
    subject: str          # from JWT sub claim (actor)
    resource: str         # resource kind, e.g. "DataResource"
    resource_name: str    # specific resource name
    action: str           # "create", "delete", "restore"
    outcome: str          # "success" or "failure"
    request_id: str
    timestamp: str = field(default_factory=lambda: datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))


def _build_payload(event: AuditEvent) -> bytes:
    """Serialize AuditEvent to the audit service wire format."""
    payload = {
        "tenant": event.tenant,
        "actor": event.subject,
        "action": event.action,
        "resource": event.resource,
        "outcome": event.outcome,
        "details": {
            "eventType": event.event_type,
            "resourceName": event.resource_name,
            "requestId": event.request_id,
        },
        "timestamp": event.timestamp,
    }
    return json.dumps(payload).encode("utf-8")


def _post_audit(url: str, payload: bytes) -> None:
    """Blocking HTTP POST — intended to be wrapped in asyncio.to_thread()."""
    req = urllib.request.Request(
        url,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            status = resp.status
            if status >= 400:
                logger.warning("audit service returned %s", status)
    except urllib.error.URLError as exc:
        logger.warning("audit emit failed: %s", exc)
    except Exception as exc:  # noqa: BLE001
        logger.warning("audit emit unexpected error: %s", exc)


async def emit_audit(event: AuditEvent) -> None:
    """Fire-and-forget POST to the audit service.

    Never raises — logs WARNING on failure and returns silently.
    Skipped entirely when AUDIT_SERVICE_URL is set to an empty string.
    """
    url = os.environ.get("AUDIT_SERVICE_URL", _DEFAULT_AUDIT_URL)
    if not url:
        return

    endpoint = url.rstrip("/") + "/api/v1/audit/events"
    payload = _build_payload(event)

    try:
        await asyncio.to_thread(_post_audit, endpoint, payload)
    except Exception as exc:  # noqa: BLE001
        logger.warning("audit emit thread error: %s", exc)
