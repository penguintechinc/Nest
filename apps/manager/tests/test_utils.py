"""Unit tests for utility modules."""
import asyncio
import json
from unittest.mock import AsyncMock, MagicMock, patch
import pytest
from utils.redis_sync import (
    sync_to_redis,
    sync_threat_intel_to_redis,
    DBLB_ROUTES_KEY,
    THREAT_INTEL_KEY,
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
                mock_redis.set.assert_called_once()
                call_args = mock_redis.set.call_args
                assert call_args[0][0] == DBLB_ROUTES_KEY
                assert call_args[0][1] == "[]"
                assert call_args[1]["ex"] == CACHE_TTL

        asyncio.run(run_test())

    def test_sync_to_redis_with_servers(self):
        """Test sync_to_redis with multiple server types."""
        servers = [
            {"id": 1, "db_type": "postgresql", "host": "pg1.local", "port": 5432},
            {"id": 2, "db_type": "mysql", "host": "mysql1.local", "port": 3306},
            {"id": 3, "db_type": "redis", "host": "redis1.local", "port": 6379},
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
                mock_redis.set.assert_called_once()
                call_args = mock_redis.set.call_args
                routes_json = json.loads(call_args[0][1])
                assert len(routes_json) == 3
                assert "postgresql:5432:pg1.local:5432" in routes_json
                assert "mysql:3306:mysql1.local:3306" in routes_json
                assert "redis:6380:redis1.local:6379" in routes_json

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

    def test_sync_threat_intel_to_redis_no_indicators(self):
        """Test sync_threat_intel_to_redis with no indicators."""
        async def run_test():
            with patch("asyncio.to_thread") as mock_to_thread:
                mock_to_thread.return_value = []
                with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                    mock_redis = AsyncMock()
                    mock_redis_getter.return_value = mock_redis
                    mock_redis.__aenter__.return_value = mock_redis
                    mock_redis.__aexit__.return_value = None

                    await sync_threat_intel_to_redis(MagicMock())
                    mock_redis.set.assert_called_once()
                    call_args = mock_redis.set.call_args
                    assert call_args[0][0] == THREAT_INTEL_KEY
                    assert call_args[0][1] == "{}"
                    assert call_args[1]["ex"] == CACHE_TTL

        asyncio.run(run_test())

    def test_sync_threat_intel_to_redis_with_indicators(self):
        """Test sync_threat_intel_to_redis with mixed indicator types."""
        indicators = [
            {"id": 1, "indicator_type": "ip", "value": "192.168.1.1", "feed_id": 1},
            {"id": 2, "indicator_type": "domain", "value": "malware.local", "feed_id": 1},
            {"id": 3, "indicator_type": "ip", "value": "10.0.0.1", "feed_id": 1},
            {"id": 4, "indicator_type": "hash", "value": "abc123def", "feed_id": 1},
        ]

        async def run_test():
            with patch("asyncio.to_thread") as mock_to_thread:
                mock_to_thread.return_value = indicators
                with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                    mock_redis = AsyncMock()
                    mock_redis_getter.return_value = mock_redis
                    mock_redis.__aenter__.return_value = mock_redis
                    mock_redis.__aexit__.return_value = None

                    await sync_threat_intel_to_redis(MagicMock())
                    mock_redis.set.assert_called_once()
                    call_args = mock_redis.set.call_args
                    by_type = json.loads(call_args[0][1])
                    assert len(by_type) == 3
                    assert len(by_type["ip"]) == 2
                    assert "192.168.1.1" in by_type["ip"]
                    assert "10.0.0.1" in by_type["ip"]
                    assert len(by_type["domain"]) == 1
                    assert "malware.local" in by_type["domain"]
                    assert len(by_type["hash"]) == 1

        asyncio.run(run_test())

    def test_sync_threat_intel_to_redis_expired_indicators_excluded(self):
        """Test that expired threat intel indicators are excluded."""
        from datetime import datetime, timezone, timedelta

        now = datetime.now(timezone.utc)
        valid_indicators = [
            {"id": 1, "indicator_type": "ip", "value": "192.168.1.1", "expires_at": None},
            {"id": 3, "indicator_type": "domain", "value": "safe.local", "expires_at": now + timedelta(days=1)},
        ]

        async def run_test():
            with patch("asyncio.to_thread") as mock_to_thread:
                mock_to_thread.return_value = valid_indicators
                with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                    mock_redis = AsyncMock()
                    mock_redis_getter.return_value = mock_redis
                    mock_redis.__aenter__.return_value = mock_redis
                    mock_redis.__aexit__.return_value = None

                    await sync_threat_intel_to_redis(MagicMock())
                    call_args = mock_redis.set.call_args
                    by_type = json.loads(call_args[0][1])
                    assert "ip" in by_type
                    assert "domain" in by_type
                    assert len(by_type["ip"]) == 1

        asyncio.run(run_test())

    def test_sync_threat_intel_to_redis_unknown_type_handling(self):
        """Test that unknown indicator types are handled."""
        indicators = [
            {"id": 1, "indicator_type": "unknown_type", "value": "val1"},
            {"id": 2, "value": "val2"},
        ]

        async def run_test():
            with patch("asyncio.to_thread") as mock_to_thread:
                mock_to_thread.return_value = indicators
                with patch("utils.redis_sync.get_redis") as mock_redis_getter:
                    mock_redis = AsyncMock()
                    mock_redis_getter.return_value = mock_redis
                    mock_redis.__aenter__.return_value = mock_redis
                    mock_redis.__aexit__.return_value = None

                    await sync_threat_intel_to_redis(MagicMock())
                    call_args = mock_redis.set.call_args
                    by_type = json.loads(call_args[0][1])
                    assert "unknown_type" in by_type
                    assert "unknown" in by_type
                    assert "val1" in by_type["unknown_type"]
                    assert "val2" in by_type["unknown"]

        asyncio.run(run_test())
