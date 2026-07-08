"""Nest API entry point."""

import asyncio
import logging
import os
import sys

# Add current directory to path so imports work
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from logging_config import configure_logging

configure_logging()

from app import create_app
from db_init import init_db
from grpc_server import start_grpc_in_background
from store import MemoryStore, SQLStore

logger = logging.getLogger(__name__)


async def main():
    """Main entry point."""
    # Start gRPC server in background
    start_grpc_in_background()
    logger.info("gRPC server started in background")

    # Create and start Quart app
    # Select store based on environment: use SQLStore if DB_TYPE is set,
    # otherwise fall back to MemoryStore for testing/local dev
    use_sql_store = os.getenv("USE_SQL_STORE", "").lower() in (
        "true",
        "1",
        "yes",
    ) or os.getenv("DB_TYPE")

    if use_sql_store:
        logger.info("Initializing SQLStore with durable database backend")
        init_db()
        store = SQLStore.create_from_env()
    else:
        logger.info("Using in-memory MemoryStore (testing/local development)")
        store = MemoryStore()

    app = create_app(store)

    port = int(os.getenv("PORT", "8080"))
    logger.info(f"Nest API server starting on :{port}")

    # Run with hypercorn
    from hypercorn.asyncio import serve
    from hypercorn.config import Config

    config = Config()
    config.bind = [f"0.0.0.0:{port}"]
    config.access_log_format = '%(h)s %(l)s %(u)s %(t)s "%(r)s" %(s)s %(b)s "%(q)s"'

    await serve(app, config)


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Shutting down")
        sys.exit(0)
