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
from grpc_server import start_grpc_in_background
from store import MemoryStore

logger = logging.getLogger(__name__)


async def main():
    """Main entry point."""
    # Start gRPC server in background
    start_grpc_in_background()
    logger.info("gRPC server started in background")

    # Create and start Quart app
    store = MemoryStore()
    app = create_app(store)

    port = int(os.getenv("PORT", "8080"))
    logger.info(f"Nest API server starting on :{port}")

    # Run with hypercorn
    from hypercorn.asyncio import serve
    from hypercorn.config import Config

    config = Config()
    config.bind = [f"0.0.0.0:{port}"]
    config.access_log_format = (
        '%(h)s %(l)s %(u)s %(t)s "%(r)s" %(s)s %(b)s "%(q)s"'
    )

    await serve(app, config)


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Shutting down")
        sys.exit(0)
