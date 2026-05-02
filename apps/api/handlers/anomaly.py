"""Anomaly handler — proxies to nest-anomaly-detector service (enterprise/WaddleAI gated)."""
import os
import uuid
from typing import Any

import aiohttp
from quart import jsonify, request, g

# NOTE: The anomaly-detector currently exposes gRPC on :50061 only; the HTTP mux
# exists in mux.go but is not yet wired to a listener. Until that is fixed, this
# handler will gracefully return an empty list with status "unavailable".
# Once the HTTP server is wired, set ANOMALY_DETECTOR_URL to the HTTP endpoint.
_ANOMALY_DETECTOR_URL = os.environ.get(
    "ANOMALY_DETECTOR_URL",
    "http://nest-anomaly-detector.nest.svc.cluster.local:8092",
)

_TIMEOUT = aiohttp.ClientTimeout(total=10)


async def _get(path: str, params: dict | None = None) -> tuple[Any, int]:
    """Make a GET request to the anomaly-detector service.

    Args:
        path: URL path (must start with /).
        params: Optional query parameters.

    Returns:
        Tuple of (parsed JSON body, HTTP status code).
    """
    url = f"{_ANOMALY_DETECTOR_URL.rstrip('/')}{path}"
    try:
        async with aiohttp.ClientSession(timeout=_TIMEOUT) as session:
            async with session.get(url, params=params) as resp:
                body = await resp.json(content_type=None)
                return body, resp.status
    except aiohttp.ClientConnectorError:
        return None, 503
    except Exception:
        return None, 502


async def list_anomalies(tenant_id: str):
    """Return current anomalies for a tenant's DataResources.

    GET /api/v1/tenants/{tid}/anomalies
    Proxies to GET /api/v1/anomaly/current?tenant={tid} on the anomaly-detector service.

    Query params forwarded: severity, limit.
    """
    request_id = getattr(g, "request_id", str(uuid.uuid4()))

    params: dict[str, str] = {"tenant": tenant_id}
    severity = request.args.get("severity")
    limit = request.args.get("limit")
    if severity:
        params["severity"] = severity
    if limit:
        params["limit"] = limit

    body, status = await _get("/api/v1/anomaly/current", params=params)

    if body is None:
        # Service unreachable — return empty list so callers degrade gracefully
        return (
            jsonify(
                {
                    "anomalies": [],
                    "status": "unavailable",
                    "tenantId": tenant_id,
                    "meta": {"version": 1},
                }
            ),
            200,
        )

    if status == 402:
        # WaddleAI/enterprise license not configured — propagate clearly
        return (
            jsonify(
                {
                    "code": "nest.enterprise.license_required",
                    "message": "Anomaly detection requires an enterprise license with WaddleAI enabled",
                    "requestId": request_id,
                    "docsUrl": "https://docs.nest.penguintech.io/errors/nest.enterprise.license_required",
                }
            ),
            402,
        )

    if status != 200:
        return jsonify(body), status

    return (
        jsonify(
            {
                "status": "ok",
                "tenantId": tenant_id,
                "anomalies": body.get("anomalies", []),
                "count": body.get("count", 0),
                "meta": {"version": 1},
            }
        ),
        200,
    )
