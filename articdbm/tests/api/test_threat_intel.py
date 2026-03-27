"""Tests for /api/threat-intel endpoints."""
import os
import pytest
import requests

MANAGER_URL = os.getenv("MANAGER_URL", "http://localhost:8000")

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1",
)


@skip_if_no_services
class TestThreatIntelFeeds:
    def test_list_feeds_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/threat-intel/feeds", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_feeds(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/threat-intel/feeds", timeout=10)
        assert resp.status_code == 200

    def test_create_feed(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/threat-intel/feeds",
            json={
                "name": "test-feed-api",
                "type": "custom",
                "polling_interval": 3600,
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201, 403, 400)


@skip_if_no_services
class TestThreatIntelIndicators:
    def test_list_indicators_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/threat-intel/indicators", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_indicators(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/threat-intel/indicators", timeout=10)
        assert resp.status_code == 200

    def test_create_indicator(self, api_session, api_url):
        resp = api_session.post(
            f"{api_url}/threat-intel/indicators",
            json={
                "indicator_type": "ip",
                "value": "10.0.0.99",
                "threat_level": "high",
                "confidence": 80,
                "description": "API test indicator",
            },
            timeout=10,
        )
        assert resp.status_code in (200, 201)


@skip_if_no_services
class TestThreatIntelMatches:
    def test_list_matches_requires_auth(self, anon_session):
        resp = anon_session.get(f"{MANAGER_URL}/api/threat-intel/matches", timeout=10)
        assert resp.status_code in (401, 403)

    def test_list_matches(self, api_session, api_url):
        resp = api_session.get(f"{api_url}/threat-intel/matches", timeout=10)
        assert resp.status_code == 200
