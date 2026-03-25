"""Tests for /api/cloud-providers and related endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestCloudProviders:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/cloud-providers", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_providers(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/cloud-providers", timeout=10)
        assert resp.status_code == 200

    def test_create_provider(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/cloud-providers",
            json={
                "name": "test-k8s",
                "provider_type": "kubernetes",
                "configuration": {"context": "test"},
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201, 400)


@skip_if_no_services
class TestCloudInstances:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/cloud-instances", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_instances(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/cloud-instances", timeout=10)
        assert resp.status_code == 200


@skip_if_no_services
class TestScalingPolicies:
    def test_list_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/scaling-policies", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_policies(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/scaling-policies", timeout=10)
        assert resp.status_code == 200
