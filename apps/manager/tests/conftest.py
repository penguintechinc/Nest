"""Pytest configuration and fixtures."""

import os
import tempfile

# Set default test environment variables before importing anything
# Create temporary directory for SQLite database file
_test_db_dir = tempfile.mkdtemp(prefix="pytest_manager_")
_test_db_path = os.path.join(_test_db_dir, "test.db")

os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("DB_HOST", "localhost")
os.environ.setdefault("DB_PORT", "5432")
os.environ.setdefault("DB_NAME", _test_db_path)
os.environ.setdefault("DB_USER", "test")
os.environ.setdefault("DB_PASS", "test")

import pytest
from sqlalchemy import MetaData, Table, Column, String, Integer, Text, DateTime, create_engine

from app import create_app
from models.operations import OperationRecord
from penguin_dal import AsyncDB
from store import MemoryOperationStore, SQLOperationStore


@pytest.fixture
async def app():
    """Create a Quart test app."""
    app = create_app()
    app.config["TESTING"] = True
    return app


@pytest.fixture
async def client(app):
    """Create a test client."""
    return app.test_client()


@pytest.fixture
async def memory_store():
    """Create an in-memory store for testing."""
    return MemoryOperationStore()


@pytest.fixture
async def sql_store():
    """Create a SQL-backed store for testing with real SQLite database."""
    # Create a temporary SQLite database
    with tempfile.NamedTemporaryFile(suffix=".db", delete=False) as f:
        db_path = f.name

    try:
        # Create the schema using SQLAlchemy (idempotent via create_all)
        db_uri = f"sqlite+aiosqlite:///{db_path}"
        sync_engine = create_engine(f"sqlite:///{db_path}")
        metadata = MetaData()

        Table(
            "operations",
            metadata,
            Column("tenant", String(255), primary_key=True, nullable=False),
            Column("id", String(36), primary_key=True),
            Column("resource_name", String(255), nullable=False),
            Column("resource_type", String(100), nullable=False),
            Column("operation_type", String(100), nullable=False),
            Column("phase", String(50), nullable=False, index=True),
            Column("message", Text, nullable=False),
            Column("error", Text, nullable=True),
            Column("progress", Integer, nullable=False, default=0),
            Column("created_at", DateTime, nullable=False),
            Column("updated_at", DateTime, nullable=False),
        )
        metadata.create_all(sync_engine)
        sync_engine.dispose()

        # Create AsyncDB and reflect
        db = AsyncDB(db_uri, pool_size=5, echo=False)
        await db.reflect()
        store = SQLOperationStore(db)
        yield store
        await db.close()
    finally:
        # Clean up
        if os.path.exists(db_path):
            os.unlink(db_path)


@pytest.fixture
async def store(memory_store):
    """Default store fixture (memory)."""
    return memory_store
