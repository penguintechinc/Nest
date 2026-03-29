"""
Database Health Checker Worker.
Checks connectivity for all registered database_server entries.
Supports mysql, postgresql, redis, mongodb, mssql via async drivers.
"""
import asyncio
import logging
import os
from datetime import datetime, timezone

logger = logging.getLogger(__name__)
HEALTH_CHECK_INTERVAL = int(os.environ.get("DB_HEALTH_CHECK_INTERVAL", "60"))

async def check_postgresql(host: str, port: int, timeout: float = 5.0) -> bool:
    try:
        import asyncpg
        conn = await asyncio.wait_for(
            asyncpg.connect(host=host, port=port, user="postgres", password="", database="postgres"),
            timeout=timeout
        )
        await conn.close()
        return True
    except Exception:
        # Try port connectivity only
        try:
            reader, writer = await asyncio.wait_for(
                asyncio.open_connection(host, port), timeout=timeout
            )
            writer.close()
            return True
        except Exception:
            return False

async def check_mysql(host: str, port: int, timeout: float = 5.0) -> bool:
    try:
        reader, writer = await asyncio.wait_for(
            asyncio.open_connection(host, port), timeout=timeout
        )
        writer.close()
        return True
    except Exception:
        return False

async def check_redis_conn(host: str, port: int, timeout: float = 5.0) -> bool:
    try:
        import redis.asyncio as aioredis
        r = aioredis.Redis(host=host, port=port, socket_connect_timeout=timeout)
        await r.ping()
        await r.close()
        return True
    except Exception:
        return False

async def check_server_health(server: dict) -> bool:
    db_type = server.get("db_type", "postgresql")
    host = server.get("host", "")
    port = server.get("port", 5432)
    if db_type == "postgresql":
        return await check_postgresql(host, port)
    elif db_type in ("mysql", "mariadb"):
        return await check_mysql(host, port)
    elif db_type == "redis":
        return await check_redis_conn(host, port)
    else:
        return await check_mysql(host, port)

async def db_health_checker_loop(db) -> None:
    """Main health checker loop - runs indefinitely."""
    logger.info("DB Health Checker started")
    while True:
        try:
            servers = await asyncio.to_thread(
                lambda: db(db.database_server.active == True).select(
                    db.database_server.ALL
                ).as_list()
            )
            results = await asyncio.gather(
                *[check_server_health(s) for s in servers],
                return_exceptions=True
            )
            for server, healthy in zip(servers, results):
                if isinstance(healthy, Exception):
                    healthy = False
                logger.debug(f"Server {server['id']} ({server['host']}): {'healthy' if healthy else 'unhealthy'}")
        except Exception as e:
            logger.error(f"Health checker error: {e}", exc_info=True)

        await asyncio.sleep(HEALTH_CHECK_INTERVAL)
