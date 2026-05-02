"""Per-tenant token-bucket rate limiter."""
import asyncio
import math
import os
import time
import uuid
from dataclasses import dataclass, field

from quart import g, jsonify, request

_EXEMPT_PATHS = frozenset({"/health", "/ready", "/metrics"})

_BURST: int = int(os.getenv("RATE_LIMIT_BURST", "100"))
_PER_SECOND: float = float(os.getenv("RATE_LIMIT_PER_SECOND", "10"))


@dataclass(slots=True)
class _Bucket:
    """Token-bucket state for a single tenant."""

    tokens: float
    last_refill: float
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)


# tenant_id -> _Bucket
_buckets: dict[str, _Bucket] = {}
_registry_lock: asyncio.Lock = asyncio.Lock()


async def _get_bucket(tenant_id: str) -> _Bucket:
    """Return (or create) the bucket for *tenant_id*."""
    # Fast path — bucket already exists.
    bucket = _buckets.get(tenant_id)
    if bucket is not None:
        return bucket

    async with _registry_lock:
        # Re-check under the registry lock.
        bucket = _buckets.get(tenant_id)
        if bucket is None:
            bucket = _Bucket(tokens=float(_BURST), last_refill=time.monotonic())
            _buckets[tenant_id] = bucket
        return bucket


async def check_rate_limit() -> None:
    """Consume one token for the current tenant.

    No-op for exempt paths.  Raises a 429 Quart Response when the bucket is
    empty and stores rate-limit headers on *g* for the after_request hook.
    """
    path = request.path or "/"
    if path in _EXEMPT_PATHS:
        return

    tenant_id: str = getattr(g, "tenant", "")
    if not tenant_id:
        # Auth middleware should have already rejected the request; skip.
        return

    bucket = await _get_bucket(tenant_id)

    async with bucket.lock:
        now = time.monotonic()
        elapsed = now - bucket.last_refill
        bucket.tokens = min(float(_BURST), bucket.tokens + elapsed * _PER_SECOND)
        bucket.last_refill = now

        remaining = max(0.0, bucket.tokens - 1.0)
        # Seconds until at least one token is available.
        retry_after = math.ceil((1.0 - bucket.tokens) / _PER_SECOND) if bucket.tokens < 1.0 else 0
        reset_epoch = int(time.time() + retry_after)

        # Store for after_request hook regardless of outcome.
        g.rl_limit = _BURST
        g.rl_remaining = int(remaining)
        g.rl_reset = reset_epoch

        if bucket.tokens < 1.0:
            request_id: str = getattr(g, "request_id", str(uuid.uuid4()))
            response = await jsonify(
                {
                    "code": "nest.rate_limit.exceeded",
                    "message": "Rate limit exceeded",
                    "requestId": request_id,
                    "retryAfterSeconds": retry_after,
                }
            )
            response.status_code = 429
            response.headers["X-RateLimit-Limit"] = str(_BURST)
            response.headers["X-RateLimit-Remaining"] = "0"
            response.headers["X-RateLimit-Reset"] = str(reset_epoch)
            raise response  # type: ignore[misc]

        bucket.tokens -= 1.0
        # Update remaining after consumption.
        g.rl_remaining = int(bucket.tokens)
