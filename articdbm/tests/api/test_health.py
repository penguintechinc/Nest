"""Tests for /api/health endpoint."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestHealth:
    def test_health_returns_200(self):
        resp = requests.get(f"{MANAGER_URL}/api/health", timeout=10)
        assert resp.status_code == 200

    def test_health_json_shape(self):
        resp = requests.get(f"{MANAGER_URL}/api/health", timeout=10)
        data = resp.json()
        assert "status" in data
        assert "timestamp" in data

    def test_health_status_ok(self):
        resp = requests.get(f"{MANAGER_URL}/api/health", timeout=10)
        data = resp.json()
        assert data["status"] in ("ok", "healthy", "running")
