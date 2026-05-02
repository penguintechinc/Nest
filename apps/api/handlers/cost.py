"""Cost report handler — proxies to nest-cost-calculator service."""
import os
import uuid
from typing import Any

import aiohttp
from quart import jsonify, g

_COST_CALCULATOR_URL = os.environ.get(
    "COST_CALCULATOR_URL",
    "http://nest-cost-calculator.nest.svc.cluster.local:8091",
)

_TIMEOUT = aiohttp.ClientTimeout(total=10)


async def _get(path: str) -> tuple[Any, int]:
    """Make a GET request to the cost-calculator service.

    Args:
        path: URL path (must start with /).

    Returns:
        Tuple of (parsed JSON body, HTTP status code).
    """
    url = f"{_COST_CALCULATOR_URL.rstrip('/')}{path}"
    try:
        async with aiohttp.ClientSession(timeout=_TIMEOUT) as session:
            async with session.get(url) as resp:
                body = await resp.json(content_type=None)
                return body, resp.status
    except aiohttp.ClientConnectorError:
        return None, 503
    except Exception:
        return None, 502


async def get_cost_report(tenant_id: str):
    """Return billing history for a tenant.

    GET /api/v1/tenants/{tid}/cost-report
    Proxies to GET /api/v1/billing/{tenantId} on the cost-calculator service.
    """
    request_id = getattr(g, "request_id", str(uuid.uuid4()))
    body, status = await _get(f"/api/v1/billing/{tenant_id}")

    if body is None:
        return (
            jsonify(
                {
                    "status": "unavailable",
                    "message": "Cost calculator service is unreachable",
                    "requestId": request_id,
                    "docsUrl": "https://docs.nest.penguintech.io/errors/nest.cost.unavailable",
                }
            ),
            503,
        )

    if status != 200:
        return jsonify(body), status

    return (
        jsonify(
            {
                "status": "ok",
                "tenantId": tenant_id,
                "data": body,
                "meta": {"version": 1},
            }
        ),
        200,
    )


async def get_cost_summary(tenant_id: str):
    """Return aggregate billing summary for a tenant.

    GET /api/v1/tenants/{tid}/cost-report/summary
    Proxies to GET /api/v1/billing/{tenantId}/summary on the cost-calculator.
    """
    request_id = getattr(g, "request_id", str(uuid.uuid4()))
    body, status = await _get(f"/api/v1/billing/{tenant_id}/summary")

    if body is None:
        return (
            jsonify(
                {
                    "status": "unavailable",
                    "message": "Cost calculator service is unreachable",
                    "requestId": request_id,
                    "docsUrl": "https://docs.nest.penguintech.io/errors/nest.cost.unavailable",
                }
            ),
            503,
        )

    if status != 200:
        return jsonify(body), status

    return (
        jsonify(
            {
                "status": "ok",
                "tenantId": tenant_id,
                "data": body,
                "meta": {"version": 1},
            }
        ),
        200,
    )
