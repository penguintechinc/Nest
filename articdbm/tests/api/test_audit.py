"""Tests for /api/audit-log endpoint."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestAuditLog:
    def test_audit_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/audit-log", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_audit_log(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/audit-log", timeout=10)
        assert resp.status_code == 200
        data = resp.json()
        assert isinstance(data, (list, dict))
