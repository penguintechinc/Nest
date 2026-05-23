"""Unit tests for health check endpoint."""
import pytest
import sys
import os

# Add apps/manager to path so imports resolve
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'apps', 'manager'))

from apps.manager.app import app


@pytest.fixture
def client():
    """Create a Quart test client."""
    app.config['TESTING'] = True
    return app.test_client()


@pytest.mark.asyncio
async def test_healthz_returns_200(client):
    """Health endpoint returns 200 with healthy status."""
    response = await client.get('/healthz')
    assert response.status_code == 200
    data = await response.get_json()
    assert data['status'] == 'healthy'


@pytest.mark.asyncio
async def test_healthz_returns_json(client):
    """Health endpoint returns valid JSON content type."""
    response = await client.get('/healthz')
    assert 'application/json' in response.content_type


@pytest.mark.asyncio
async def test_not_found_returns_404(client):
    """Non-existent route returns 404 JSON error."""
    response = await client.get('/nonexistent-route')
    assert response.status_code == 404
    data = await response.get_json()
    assert 'error' in data
    assert data['error'] == 'Not found'
