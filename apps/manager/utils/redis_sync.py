"""Redis synchronization utilities for DB Proxy configuration."""
import asyncio
import json
import logging
import os
import redis.asyncio as aioredis

logger = logging.getLogger(__name__)

REDIS_HOST = os.environ.get("REDIS_HOST", "redis")
REDIS_PORT = int(os.environ.get("REDIS_PORT", "6379"))
REDIS_PASSWORD = os.environ.get("REDIS_PASSWORD", "")
DB_PROXY_REDIS_PREFIX = os.environ.get("DB_PROXY_REDIS_PREFIX", "nest:dbproxy")
DB_PROXY_ROUTES_KEY = f"{DB_PROXY_REDIS_PREFIX}:routes"
DB_PROXY_SECURITY_KEY = f"{DB_PROXY_REDIS_PREFIX}:security"
DB_PROXY_CONFIG_UPDATED_CHANNEL = f"{DB_PROXY_REDIS_PREFIX}:config:updated"
CACHE_TTL = 300


async def get_redis():
    return aioredis.Redis(
        host=REDIS_HOST, port=REDIS_PORT, password=REDIS_PASSWORD or None,
        decode_responses=True
    )


async def sync_to_redis(db) -> None:
    """Sync database_server rows to Redis in DB Proxy route format (V2 schema with read-write routing)."""
    servers = await asyncio.to_thread(
        lambda: db(db.database_server.active is True).select(
            db.database_server.ALL
        ).as_list()
    )

    routes = {}
    base_listen_ports = {"mysql": 3306, "postgresql": 5432, "mssql": 1433, "mongodb": 27017, "redis": 6380}

    for s in servers:
        db_type = s.get("db_type", "postgresql")
        host = s.get("host", "localhost")
        port = s.get("port", base_listen_ports.get(db_type, 5432))
        tenant_id = s.get("tenant_id", "default")
        max_connections = s.get("max_connections", 20)

        # Build route ID from db_type and host
        route_id = f"{tenant_id}-{host}-{db_type}"

        # Build primary endpoint
        primary_endpoint = {
            "name": "primary",
            "backend": host,
            "port": port,
            "max_connections": max_connections,
        }

        # Build replicas list (if available from read_replicas field)
        replicas = []
        read_replicas = s.get("read_replicas", [])
        if read_replicas:
            for i, replica in enumerate(read_replicas, 1):
                replica_endpoint = {
                    "name": f"replica-{i}",
                    "backend": replica.get("host", host),
                    "port": replica.get("port", port),
                    "max_connections": replica.get("max_connections", 10),
                }
                replicas.append(replica_endpoint)

        # Build complete route
        route = {
            "protocol": db_type,
            "tenant": tenant_id,
            "primary": primary_endpoint,
        }
        if replicas:
            route["replicas"] = replicas
        else:
            route["replicas"] = []

        routes[route_id] = route

    # Wrap in routes envelope
    routes_config = {"routes": routes}

    r = await get_redis()
    async with r:
        # Write the new routes config
        await r.set(DB_PROXY_ROUTES_KEY, json.dumps(routes_config), ex=CACHE_TTL)
        # Publish config update notification
        await r.publish(DB_PROXY_CONFIG_UPDATED_CHANNEL, "routes")
        logger.info(f"Synced {len(routes)} routes to Redis key {DB_PROXY_ROUTES_KEY}")


async def sync_security_config_to_redis(db) -> None:
    """Sync blocked database resources to DB Proxy security configuration."""
    blocked_resources = await asyncio.to_thread(
        lambda: db(db.blocked_database.id > 0).select(
            db.blocked_database.db_name
        ).as_list()
    )

    blocked_list = [row.get("db_name", "") for row in blocked_resources if row.get("db_name")]

    security_config = {
        "blocked_resources": blocked_list,
        "allowed_resources": [],
        "enable_injection_check": True,
    }

    r = await get_redis()
    async with r:
        # Write the security config
        await r.set(DB_PROXY_SECURITY_KEY, json.dumps(security_config), ex=CACHE_TTL)
        # Publish config update notification
        await r.publish(DB_PROXY_CONFIG_UPDATED_CHANNEL, "security")
        logger.info(f"Synced {len(blocked_list)} blocked resources to Redis key {DB_PROXY_SECURITY_KEY}")


async def sync_threat_intel_to_redis(db) -> None:
    """Sync threat intelligence indicators from database to Redis cache."""
    indicators = await asyncio.to_thread(
        lambda: db(db.threat_intel_indicator.id > 0).select(
            db.threat_intel_indicator.ALL
        ).as_list()
    )

    indicators_list = []
    for row in indicators:
        indicator = {
            "id": row.get("id"),
            "feed_id": row.get("feed_id"),
            "indicator_type": row.get("indicator_type"),
            "value": row.get("value"),
            "confidence": row.get("confidence"),
            "severity": row.get("severity"),
            "created_at": str(row.get("created_at", "")),
        }
        indicators_list.append(indicator)

    threat_intel_key = f"{DB_PROXY_REDIS_PREFIX}:threat_intel"
    r = await get_redis()
    async with r:
        await r.set(threat_intel_key, json.dumps({"indicators": indicators_list}), ex=CACHE_TTL)
        logger.info(f"Synced {len(indicators_list)} threat intel indicators to Redis key {threat_intel_key}")
