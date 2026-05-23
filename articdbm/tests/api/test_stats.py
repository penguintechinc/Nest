"""Tests for /api/stats endpoint."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestStats:
    def test_stats_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/stats", timeout=10)
        assert resp.status_code in (401, 403)

    def test_get_stats(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/stats", timeout=10)
        assert resp.status_code == 200
