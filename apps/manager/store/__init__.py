"""Operation storage layer.

Provides OperationStore implementations:
- MemoryOperationStore: In-memory, suitable for testing
- SQLOperationStore: Durable, backed by SQL database
"""

import os
from typing import Any

from penguin_dal import AsyncDB

from store.store import MemoryOperationStore, OperationStore
from store.sql_store import SQLOperationStore


async def create_operation_store() -> OperationStore:
    """Factory function to create appropriate operation store.

    Selection based on DB_TYPE environment variable:
    - If DB_TYPE is not set or 'memory': returns MemoryOperationStore
    - Otherwise: returns SQLOperationStore with AsyncDB connection

    Returns:
        OperationStore implementation (Memory or SQL based on config).

    Raises:
        ValueError: If database connection fails.
    """
    db_type = os.getenv("DB_TYPE", "memory").lower()

    if db_type == "memory":
        return MemoryOperationStore()

    # SQL-backed store — build connection string from env vars
    db_host = os.getenv("DB_HOST", "localhost")
    db_port = os.getenv("DB_PORT", "5432")
    db_name = os.getenv("DB_NAME", "nest")
    db_user = os.getenv("DB_USER", "nest")
    db_pass = os.getenv("DB_PASS", "nest")
    db_pool_size = int(os.getenv("DB_POOL_SIZE", "10"))

    # Map DB_TYPE to SQLAlchemy dialect
    dialect_map = {
        "postgresql": "postgresql+asyncpg",
        "mysql": "mysql+aiomysql",
        "sqlite": "sqlite+aiosqlite",
    }
    dialect = dialect_map.get(db_type)
    if not dialect:
        raise ValueError(
            f"Unsupported DB_TYPE: {db_type}. "
            f"Supported: {', '.join(dialect_map.keys())}"
        )

    # Build connection URI
    if db_type == "sqlite":
        uri = f"{dialect}:///{db_name}"
    else:
        uri = f"{dialect}://{db_user}:{db_pass}@{db_host}:{db_port}/{db_name}"

    # Create and initialize AsyncDB
    db = AsyncDB(uri, pool_size=db_pool_size, echo=False)
    await db.reflect()

    return SQLOperationStore(db)


__all__ = [
    "create_operation_store",
    "MemoryOperationStore",
    "SQLOperationStore",
    "OperationStore",
]
