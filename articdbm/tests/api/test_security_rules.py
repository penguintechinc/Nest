"""Tests for /api/security-rules endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestSecurityRules:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/security-rules", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_rules(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/security-rules", timeout=10)
        assert resp.status_code == 200

    def test_create_rule(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/security-rules",
            json={
                "name": "test-rule-api",
                "pattern": "DROP TABLE",
                "action": "block",
                "severity": "critical",
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201)
