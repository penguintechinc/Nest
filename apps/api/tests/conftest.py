"""Pytest fixtures for Nest API tests."""
import sys
import os
from pathlib import Path

import pytest

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from app import create_app
from store.store import MemoryStore


@pytest.fixture
async def client():
    """Create a test app with fresh MemoryStore."""
    store = MemoryStore()
    app = create_app(store)

    async with app.test_client() as test_client:
        yield test_client


@pytest.fixture
async def store():
    """Create a fresh MemoryStore for each test."""
    return MemoryStore()


@pytest.fixture
def bearer_token():
    """Generate a valid bearer token for testing.

    Format: sub:tenant:tier
    """
    return "test-sub:test-tenant:pro"


@pytest.fixture
def free_tier_token():
    """Generate a free-tier bearer token for testing."""
    return "test-sub:test-tenant:free"
