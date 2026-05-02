"""Structured JSON logging configuration for Nest API."""
import logging
import os

from pythonjsonlogger.jsonlogger import JsonFormatter


def configure_logging() -> None:
    """Configure root logger with structured JSON output.

    Reads LOG_LEVEL env var (default: INFO).
    Format includes: timestamp, level, logger, message.
    """
    log_level = os.getenv("LOG_LEVEL", "INFO").upper()

    handler = logging.StreamHandler()
    formatter = JsonFormatter(
        fmt="%(asctime)s %(levelname)s %(name)s %(message)s",
        rename_fields={
            "asctime": "timestamp",
            "levelname": "level",
            "name": "logger",
        },
    )
    handler.setFormatter(formatter)

    root_logger = logging.getLogger()
    root_logger.handlers.clear()
    root_logger.addHandler(handler)
    root_logger.setLevel(getattr(logging, log_level, logging.INFO))
