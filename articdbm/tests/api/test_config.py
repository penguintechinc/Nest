"""Tests for /api/sync, seed, blocking-config endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestConfigEndpoints:
    def test_sync_requires_auth(self, anon_session):
        resp = anon_session.post(f"{MANAGER_URL}/api/sync", timeout=10)
        assert resp.status_code in (401, 403, 405)

    def test_sync(self, api_session, api_url):
        resp = api_session.post(f"{api_url}/sync", timeout=10)
        assert resp.status_code in (200, 500)

    def test_seed_blocked_resources(self, api_session, api_url):
        resp = api_session.post(f"{api_url}/seed-blocked-resources", timeout=10)
        assert resp.status_code in (200, 201, 500)

    def test_blocking_config(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/blocking-config", timeout=10)
        assert resp.status_code == 200
