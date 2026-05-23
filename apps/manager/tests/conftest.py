"""Pytest configuration and fixtures."""

import pytest
from app import create_app
from store.store import MemoryOperationStore


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
async def store():
    """Create an in-memory store for testing."""
    return MemoryOperationStore()
