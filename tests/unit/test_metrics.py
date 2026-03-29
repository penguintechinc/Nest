"""Unit tests for Prometheus metrics endpoint."""
import pytest
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'apps', 'manager'))

from apps.manager.app import app


@pytest.fixture
def client():
    """Create a Quart test client."""
    app.config['TESTING'] = True
    return app.test_client()


@pytest.mark.asyncio
async def test_metrics_returns_200(client):
    """Metrics endpoint returns 200."""
    response = await client.get('/metrics')
    assert response.status_code == 200


@pytest.mark.asyncio
async def test_metrics_returns_prometheus_format(client):
    """Metrics endpoint returns text/plain content type for Prometheus."""
    response = await client.get('/metrics')
    assert 'text/plain' in response.content_type


@pytest.mark.asyncio
async def test_metrics_contains_http_requests_total(client):
    """Metrics output contains the http_requests_total counter."""
    response = await client.get('/metrics')
    body = (await response.get_data()).decode('utf-8')
    assert 'http_requests_total' in body


@pytest.mark.asyncio
async def test_metrics_contains_http_request_duration(client):
    """Metrics output contains the http_request_duration_seconds histogram."""
    response = await client.get('/metrics')
    body = (await response.get_data()).decode('utf-8')
    assert 'http_request_duration_seconds' in body


@pytest.mark.asyncio
async def test_metrics_increments_after_request(client):
    """After hitting healthz, metrics should reflect the request."""
    await client.get('/healthz')
    response = await client.get('/metrics')
    body = (await response.get_data()).decode('utf-8')
    # Should have at least one entry for /healthz
    assert '/healthz' in body
