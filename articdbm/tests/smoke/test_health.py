"""Smoke tests for service health endpoints."""
import os
import time
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")
PROXY_METRICS_URL = os.getenv("PROXY_METRICS_URL", "http://localhost:9091")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1 to run health checks (requires running services)",
)


def _retry_get(url, retries=5, backoff=2, timeout=10):
    """GET with retry and exponential backoff."""
    last_exc = None
    for attempt in range(retries):
        try:
            resp = requests.get(url, timeout=timeout)
            return resp
        except requests.RequestException as e:
            last_exc = e
            time.sleep(backoff * (2 ** attempt))
    raise last_exc


@skip_if_no_services
class TestManagerHealth:
    """Manager service health checks."""

    def test_manager_health_status(self):
        """GET /api/health should return 200."""
        resp = _retry_get(f"{MANAGER_URL}/api/health")
        assert resp.status_code == 200

    def test_manager_health_body(self):
        """Health response should contain status and timestamp."""
        resp = _retry_get(f"{MANAGER_URL}/api/health")
        data = resp.json()
        assert "status" in data
        assert "timestamp" in data


@skip_if_no_services
class TestProxyHealth:
    """Proxy service health checks."""

    def test_proxy_health(self):
        """GET /health should return 200."""
        resp = _retry_get(f"{PROXY_METRICS_URL}/health")
        assert resp.status_code == 200

    def test_proxy_health_body(self):
        """Health response body should be OK."""
        resp = _retry_get(f"{PROXY_METRICS_URL}/health")
        assert resp.text.strip().upper() == "OK"

    def test_proxy_metrics_available(self):
        """GET /metrics should return 200."""
        resp = _retry_get(f"{PROXY_METRICS_URL}/metrics")
        assert resp.status_code == 200

    def test_proxy_metrics_contains_key_metric(self):
        """Metrics should include articdbm_active_connections."""
        resp = _retry_get(f"{PROXY_METRICS_URL}/metrics")
        assert "articdbm_active_connections" in resp.text
