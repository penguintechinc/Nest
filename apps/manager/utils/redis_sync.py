"""Redis synchronization utilities for DBLB configuration."""
import asyncio
import json
import logging
import os
import redis.asyncio as aioredis

logger = logging.getLogger(__name__)

REDIS_HOST = os.environ.get("REDIS_HOST", "redis")
REDIS_PORT = int(os.environ.get("REDIS_PORT", "6379"))
REDIS_PASSWORD = os.environ.get("REDIS_PASSWORD", "")
DBLB_ROUTES_KEY = "marchproxy:dblb:routes"
THREAT_INTEL_KEY = "marchproxy:dblb:threat_intel"
CACHE_TTL = 300


async def get_redis():
    return aioredis.Redis(
        host=REDIS_HOST, port=REDIS_PORT, password=REDIS_PASSWORD or None,
        decode_responses=True
    )


async def sync_to_redis(db) -> None:
    """Sync database_server rows to Redis in DBLB route format."""
    servers = await asyncio.to_thread(
        lambda: db(db.database_server.active == True).select(
            db.database_server.ALL
        ).as_list()
    )
    routes = []
    base_listen_ports = {"mysql": 3306, "postgresql": 5432, "mssql": 1433, "mongodb": 27017, "redis": 6380}
    for s in servers:
        db_type = s.get("db_type", "postgresql")
        listen_port = base_listen_ports.get(db_type, 5432)
        route = f"{db_type}:{listen_port}:{s['host']}:{s['port']}"
        routes.append(route)

    r = await get_redis()
    async with r:
        await r.set(DBLB_ROUTES_KEY, json.dumps(routes), ex=CACHE_TTL)
        logger.info(f"Synced {len(routes)} routes to Redis key {DBLB_ROUTES_KEY}")


async def sync_threat_intel_to_redis(db) -> None:
    """Sync active threat intel indicators to Redis."""
    from datetime import datetime, timezone
    now = datetime.now(timezone.utc)
    indicators = await asyncio.to_thread(
        lambda: db(
            (db.threat_intel_indicator.id > 0) &
            ((db.threat_intel_indicator.expires_at == None) |
             (db.threat_intel_indicator.expires_at > now))
        ).select(db.threat_intel_indicator.ALL).as_list()
    )
    by_type = {}
    for ind in indicators:
        t = ind.get("indicator_type", "unknown")
        by_type.setdefault(t, []).append(ind.get("value", ""))

    r = await get_redis()
    async with r:
        await r.set(THREAT_INTEL_KEY, json.dumps(by_type), ex=CACHE_TTL)
        logger.info(f"Synced {len(indicators)} threat indicators to Redis")
