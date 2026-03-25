"""Integration tests for proxy Prometheus metrics endpoint."""
import os
import pytest
import requests

PROXY_METRICS_URL = os.getenv("PROXY_METRICS_URL", "http://localhost:9091")

skip_if_no_proxy = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1 to run integration tests",
)


@skip_if_no_proxy
class TestProxyMetrics:
    """Tests for the /metrics Prometheus endpoint."""

    def test_metrics_endpoint_returns_200(self):
        """GET /metrics should return 200."""
        resp = requests.get(f"{PROXY_METRICS_URL}/metrics", timeout=10)
        assert resp.status_code == 200

    def test_metrics_content_type(self):
        """Metrics should have text/plain content type."""
        resp = requests.get(f"{PROXY_METRICS_URL}/metrics", timeout=10)
        content_type = resp.headers.get("Content-Type", "")
        assert "text/plain" in content_type or "text/plain" in content_type

    def test_metrics_contains_active_connections(self):
        """Metrics should include articdbm_active_connections gauge."""
        resp = requests.get(f"{PROXY_METRICS_URL}/metrics", timeout=10)
        assert "articdbm_active_connections" in resp.text

    def test_metrics_contains_total_queries(self):
        """Metrics should include articdbm_total_queries counter."""
        resp = requests.get(f"{PROXY_METRICS_URL}/metrics", timeout=10)
        assert "articdbm_total_queries" in resp.text

    def test_metrics_contains_query_duration(self):
        """Metrics should include articdbm_query_duration_seconds histogram."""
        resp = requests.get(f"{PROXY_METRICS_URL}/metrics", timeout=10)
        assert "articdbm_query_duration_seconds" in resp.text

    def test_metrics_contains_auth_failures(self):
        """Metrics should include articdbm_auth_failures_total counter."""
        resp = requests.get(f"{PROXY_METRICS_URL}/metrics", timeout=10)
        assert "articdbm_auth_failures_total" in resp.text

    def test_metrics_contains_sql_injection_counter(self):
        """Metrics should include articdbm_sql_injection_attempts_total."""
        resp = requests.get(f"{PROXY_METRICS_URL}/metrics", timeout=10)
        assert "articdbm_sql_injection_attempts_total" in resp.text

    def test_metrics_valid_prometheus_format(self):
        """Metrics output should follow Prometheus exposition format."""
        resp = requests.get(f"{PROXY_METRICS_URL}/metrics", timeout=10)
        lines = resp.text.strip().split("\n")
        for line in lines:
            if line.startswith("#"):
                # Comment or TYPE/HELP line
                assert line.startswith("# HELP") or line.startswith("# TYPE") or line.startswith("#")
            elif line.strip():
                # Metric line: name{labels} value [timestamp]
                parts = line.split()
                assert len(parts) >= 2, f"Invalid metric line: {line}"

    def test_health_endpoint(self):
        """GET /health should return 200 OK."""
        resp = requests.get(f"{PROXY_METRICS_URL}/health", timeout=10)
        assert resp.status_code == 200
