"""Unit tests for utility modules."""
import asyncio
import json
from unittest.mock import AsyncMock, MagicMock, patch
import pytest
from utils.redis_sync import (
    sync_to_redis,
    sync_security_config_to_redis,
    DB_PROXY_ROUTES_KEY,
    DB_PROXY_SECURITY_KEY,
    CACHE_TTL,
)


class TestRedisSyncUtils:
    """Test suite for redis_sync utility functions."""

    def test_sync_to_redis_empty_servers(self):
        """Test sync_to_redis with no active servers."""
        mock_db = MagicMock()
        mock_db.return_value.select.return_value.as_list.return_value = []

        async def run_test():
            with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                mock_redis = AsyncMock()
                mock_redis_getter.return_value = mock_redis
                mock_redis.__aenter__.return_value = mock_redis
                mock_redis.__aexit__.return_value = None

                await sync_to_redis(mock_db)
                # Should call set and publish
                assert mock_redis.set.call_count >= 1
                assert mock_redis.publish.call_count >= 1
                call_args = mock_redis.set.call_args
                assert call_args[0][0] == DB_PROXY_ROUTES_KEY
                routes_json = json.loads(call_args[0][1])
                assert routes_json == {"routes": {}}
                assert call_args[1]["ex"] == CACHE_TTL

        asyncio.run(run_test())

    def test_sync_to_redis_with_servers(self):
        """Test sync_to_redis with multiple server types."""
        servers = [
            {"id": 1, "db_type": "postgresql", "host": "pg1.local", "port": 5432, "tenant_id": "tenant-1"},
            {"id": 2, "db_type": "mysql", "host": "mysql1.local", "port": 3306, "tenant_id": "tenant-1"},
            {"id": 3, "db_type": "redis", "host": "redis1.local", "port": 6379, "tenant_id": "tenant-1"},
        ]
        mock_db = MagicMock()
        mock_db.return_value.select.return_value.as_list.return_value = servers

        async def run_test():
            with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                mock_redis = AsyncMock()
                mock_redis_getter.return_value = mock_redis
                mock_redis.__aenter__.return_value = mock_redis
                mock_redis.__aexit__.return_value = None

                await sync_to_redis(mock_db)
                assert mock_redis.set.call_count >= 1
                assert mock_redis.publish.call_count >= 1
                call_args = mock_redis.set.call_args
                routes_config = json.loads(call_args[0][1])
                assert "routes" in routes_config
                routes = routes_config["routes"]
                assert len(routes) == 3
                # Check for route entries
                assert any("pg1.local" in k for k in routes.keys())
                assert any("mysql1.local" in k for k in routes.keys())
                assert any("redis1.local" in k for k in routes.keys())
                # Check route structure
                for route_id, route in routes.items():
                    assert "protocol" in route
                    assert "tenant" in route
                    assert "primary" in route
                    assert "backend" in route["primary"]
                    assert "port" in route["primary"]

        asyncio.run(run_test())

    def test_sync_to_redis_redis_connection_error(self):
        """Test sync_to_redis handles Redis connection errors gracefully."""
        servers = [{"id": 1, "db_type": "postgresql", "host": "pg1.local", "port": 5432}]
        mock_db = MagicMock()
        mock_db.return_value.select.return_value.as_list.return_value = servers

        async def run_test():
            with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                mock_redis = AsyncMock()
                mock_redis_getter.return_value = mock_redis
                mock_redis.__aenter__.side_effect = Exception("Connection failed")
                mock_redis.__aexit__.return_value = None

                with pytest.raises(Exception):
                    await sync_to_redis(mock_db)

        asyncio.run(run_test())

    def test_sync_security_config_to_redis_no_blocked(self):
        """Test sync_security_config_to_redis with no blocked resources."""
        async def run_test():
            with patch("asyncio.to_thread") as mock_to_thread:
                mock_to_thread.return_value = []
                with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                    mock_redis = AsyncMock()
                    mock_redis_getter.return_value = mock_redis
                    mock_redis.__aenter__.return_value = mock_redis
                    mock_redis.__aexit__.return_value = None

                    await sync_security_config_to_redis(MagicMock())
                    assert mock_redis.set.call_count >= 1
                    assert mock_redis.publish.call_count >= 1
                    call_args = mock_redis.set.call_args
                    assert call_args[0][0] == DB_PROXY_SECURITY_KEY
                    config = json.loads(call_args[0][1])
                    assert config["blocked_resources"] == []
                    assert config["allowed_resources"] == []
                    assert config["enable_injection_check"] is True

        asyncio.run(run_test())

    def test_sync_security_config_to_redis_with_blocked(self):
        """Test sync_security_config_to_redis with blocked resources."""
        blocked_resources = [
            {"db_name": "information_schema"},
            {"db_name": "mysql"},
            {"db_name": "pg_catalog"},
        ]

        async def run_test():
            with patch("asyncio.to_thread") as mock_to_thread:
                mock_to_thread.return_value = blocked_resources
                with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                    mock_redis = AsyncMock()
                    mock_redis_getter.return_value = mock_redis
                    mock_redis.__aenter__.return_value = mock_redis
                    mock_redis.__aexit__.return_value = None

                    await sync_security_config_to_redis(MagicMock())
                    call_args = mock_redis.set.call_args
                    config = json.loads(call_args[0][1])
                    assert len(config["blocked_resources"]) == 3
                    assert "information_schema" in config["blocked_resources"]
                    assert "mysql" in config["blocked_resources"]
                    assert "pg_catalog" in config["blocked_resources"]

        asyncio.run(run_test())
