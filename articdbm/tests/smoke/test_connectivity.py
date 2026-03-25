"""Smoke tests for database connectivity."""
import os
import socket
import pytest

skip_if_no_services = pytest.mark.skipif(
    not os.getenv("RUN_INTEGRATION_TESTS"),
    reason="Set RUN_INTEGRATION_TESTS=1 to run connectivity tests",
)


def _tcp_connect(host, port, timeout=5):
    """Test TCP connectivity to host:port."""
    try:
        sock = socket.create_connection((host, port), timeout=timeout)
        sock.close()
        return True
    except (socket.timeout, ConnectionRefusedError, OSError):
        return False


@skip_if_no_services
class TestRedisDevConnectivity:
    """Redis dev instance connectivity."""

    def test_redis_dev_tcp(self):
        """Redis dev should accept TCP connections on port 6379."""
        host = os.getenv("REDIS_DEV_HOST", "localhost")
        port = int(os.getenv("REDIS_DEV_PORT", "6379"))
        assert _tcp_connect(host, port), f"Cannot connect to Redis dev at {host}:{port}"

    def test_redis_dev_ping(self):
        """Redis dev should respond to PING."""
        try:
            import redis
            host = os.getenv("REDIS_DEV_HOST", "localhost")
            port = int(os.getenv("REDIS_DEV_PORT", "6379"))
            r = redis.Redis(host=host, port=port, socket_timeout=5)
            assert r.ping()
        except ImportError:
            pytest.skip("redis package not installed")


@skip_if_no_services
class TestPostgresDevConnectivity:
    """PostgreSQL dev instance connectivity."""

    def test_postgres_dev_tcp(self):
        """PostgreSQL dev should accept TCP connections on port 5433."""
        host = os.getenv("POSTGRES_DEV_HOST", "localhost")
        port = int(os.getenv("POSTGRES_DEV_PORT", "5433"))
        assert _tcp_connect(host, port), f"Cannot connect to Postgres dev at {host}:{port}"


@skip_if_no_services
class TestMySQLTestConnectivity:
    """MySQL test instance connectivity."""

    def test_mysql_test_tcp(self):
        """MySQL test should accept TCP connections on port 3307."""
        host = os.getenv("MYSQL_TEST_HOST", "localhost")
        port = int(os.getenv("MYSQL_TEST_PORT", "3307"))
        assert _tcp_connect(host, port), f"Cannot connect to MySQL test at {host}:{port}"


@skip_if_no_services
class TestPostgresTestConnectivity:
    """PostgreSQL test instance connectivity."""

    def test_postgres_test_tcp(self):
        """PostgreSQL test should accept TCP connections on port 5434."""
        host = os.getenv("POSTGRES_TEST_HOST", "localhost")
        port = int(os.getenv("POSTGRES_TEST_PORT", "5434"))
        assert _tcp_connect(host, port), f"Cannot connect to Postgres test at {host}:{port}"


@skip_if_no_services
class TestMongoTestConnectivity:
    """MongoDB test instance connectivity."""

    def test_mongo_test_tcp(self):
        """MongoDB test should accept TCP connections on port 27017."""
        host = os.getenv("MONGO_TEST_HOST", "localhost")
        port = int(os.getenv("MONGO_TEST_PORT", "27017"))
        assert _tcp_connect(host, port), f"Cannot connect to MongoDB test at {host}:{port}"


@skip_if_no_services
class TestRedisTestConnectivity:
    """Redis test instance connectivity."""

    def test_redis_test_tcp(self):
        """Redis test should accept TCP connections on port 6380."""
        host = os.getenv("REDIS_TEST_HOST", "localhost")
        port = int(os.getenv("REDIS_TEST_PORT", "6380"))
        assert _tcp_connect(host, port), f"Cannot connect to Redis test at {host}:{port}"

    def test_redis_test_ping(self):
        """Redis test should respond to authenticated PING."""
        try:
            import redis
            host = os.getenv("REDIS_TEST_HOST", "localhost")
            port = int(os.getenv("REDIS_TEST_PORT", "6380"))
            password = os.getenv("REDIS_TEST_PASSWORD", "testpass123")
            r = redis.Redis(host=host, port=port, password=password, socket_timeout=5)
            assert r.ping()
        except ImportError:
            pytest.skip("redis package not installed")
