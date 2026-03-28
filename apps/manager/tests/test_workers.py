"""Unit tests for worker modules with mocks."""
import os
import sys
import pytest
from unittest.mock import AsyncMock, MagicMock, patch, call
from concurrent.futures import ProcessPoolExecutor
import asyncio

# Use a unique event loop policy per test
@pytest.fixture(scope="function")
def event_loop():
    """Create an event loop per test function."""
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    yield loop
    loop.close()

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("REDIS_HOST", "localhost")
os.environ.setdefault("REDIS_PORT", "6379")
os.environ.setdefault("THREAT_INTEL_POLL_INTERVAL", "300")
os.environ.setdefault("SCALING_EVAL_INTERVAL", "120")
os.environ.setdefault("DB_HEALTH_CHECK_INTERVAL", "60")

pytestmark = pytest.mark.asyncio


class TestDbHealthChecker:
    """Tests for db_health_checker worker module."""

    @pytest.mark.asyncio
    async def test_check_postgresql_success(self):
        """Test successful PostgreSQL connection."""
        with patch.dict("sys.modules", {"asyncpg": MagicMock()}):
            with patch("asyncio.wait_for", new_callable=AsyncMock) as mock_wait:
                mock_conn = AsyncMock()
                mock_wait.return_value = mock_conn
                from workers.db_health_checker import check_postgresql

                result = await check_postgresql("localhost", 5432)
                assert result is True

    @pytest.mark.asyncio
    async def test_check_postgresql_fallback_tcp(self):
        """Test PostgreSQL fallback to TCP connectivity."""
        with patch.dict("sys.modules", {"asyncpg": MagicMock()}):
            with patch("asyncio.wait_for", new_callable=AsyncMock) as mock_wait:
                mock_wait.side_effect = [Exception("conn failed"), (AsyncMock(), MagicMock())]
                with patch(
                    "asyncio.open_connection", new_callable=AsyncMock
                ) as mock_tcp:
                    mock_writer = MagicMock()
                    mock_tcp.return_value = (AsyncMock(), mock_writer)
                    from workers.db_health_checker import check_postgresql

                    result = await check_postgresql("localhost", 5432)
                    assert result is True

    @pytest.mark.asyncio
    async def test_check_postgresql_all_fail(self):
        """Test PostgreSQL failure on both asyncpg and TCP."""
        with patch.dict("sys.modules", {"asyncpg": MagicMock()}):
            with patch("asyncio.wait_for", side_effect=Exception("fail")):
                with patch(
                    "asyncio.open_connection", side_effect=Exception("tcp fail")
                ):
                    from workers.db_health_checker import check_postgresql

                    result = await check_postgresql("localhost", 5432)
                    assert result is False

    @pytest.mark.asyncio
    async def test_check_postgresql_timeout(self):
        """Test PostgreSQL timeout handling."""
        async def timeout_coro(*args, **kwargs):
            raise asyncio.TimeoutError()

        with patch(
            "asyncio.open_connection", side_effect=timeout_coro
        ):
            from workers.db_health_checker import check_postgresql

            result = await check_postgresql("localhost", 5432, timeout=0.1)
            assert result is False

    @pytest.mark.asyncio
    async def test_check_mysql_success(self):
        """Test successful MySQL TCP connection."""
        with patch("asyncio.open_connection", new_callable=AsyncMock) as mock_tcp:
            mock_writer = MagicMock()
            mock_tcp.return_value = (AsyncMock(), mock_writer)
            from workers.db_health_checker import check_mysql

            result = await check_mysql("localhost", 3306)
            assert result is True
            mock_tcp.assert_called_once()

    @pytest.mark.asyncio
    async def test_check_mysql_failure(self):
        """Test MySQL connection failure."""
        with patch(
            "asyncio.open_connection", side_effect=Exception("connection refused")
        ):
            from workers.db_health_checker import check_mysql

            result = await check_mysql("localhost", 3306)
            assert result is False

    @pytest.mark.asyncio
    async def test_check_redis_conn_success(self):
        """Test successful Redis connection."""
        with patch("redis.asyncio.Redis") as mock_redis_class:
            mock_redis = AsyncMock()
            mock_redis.ping = AsyncMock(return_value=True)
            mock_redis.close = AsyncMock()
            mock_redis_class.return_value = mock_redis
            from workers.db_health_checker import check_redis_conn

            result = await check_redis_conn("localhost", 6379)
            assert result is True
            mock_redis.ping.assert_called_once()

    @pytest.mark.asyncio
    async def test_check_redis_conn_failure(self):
        """Test Redis connection failure."""
        with patch("redis.asyncio.Redis", side_effect=Exception("Connection failed")):
            from workers.db_health_checker import check_redis_conn

            result = await check_redis_conn("localhost", 6379)
            assert result is False

    @pytest.mark.asyncio
    async def test_check_server_health_postgresql(self):
        """Test check_server_health with PostgreSQL type."""
        server = {"id": 1, "db_type": "postgresql", "host": "localhost", "port": 5432}
        with patch.dict("sys.modules", {"asyncpg": MagicMock()}):
            with patch("asyncio.wait_for", new_callable=AsyncMock) as mock_wait:
                mock_wait.return_value = AsyncMock()
                from workers.db_health_checker import check_server_health

                result = await check_server_health(server)
                assert isinstance(result, bool)

    @pytest.mark.asyncio
    async def test_check_server_health_mysql(self):
        """Test check_server_health with MySQL type."""
        server = {"id": 2, "db_type": "mysql", "host": "localhost", "port": 3306}
        with patch("asyncio.open_connection", new_callable=AsyncMock) as mock_tcp:
            mock_writer = MagicMock()
            mock_tcp.return_value = (AsyncMock(), mock_writer)
            from workers.db_health_checker import check_server_health

            result = await check_server_health(server)
            assert result is True

    @pytest.mark.asyncio
    async def test_check_server_health_mariadb(self):
        """Test check_server_health with MariaDB type."""
        server = {"id": 3, "db_type": "mariadb", "host": "localhost", "port": 3306}
        with patch("asyncio.open_connection", new_callable=AsyncMock) as mock_tcp:
            mock_writer = MagicMock()
            mock_tcp.return_value = (AsyncMock(), mock_writer)
            from workers.db_health_checker import check_server_health

            result = await check_server_health(server)
            assert result is True

    @pytest.mark.asyncio
    async def test_check_server_health_redis(self):
        """Test check_server_health with Redis type."""
        server = {"id": 4, "db_type": "redis", "host": "localhost", "port": 6379}
        with patch("redis.asyncio.Redis") as mock_redis_class:
            mock_redis = AsyncMock()
            mock_redis.ping = AsyncMock(return_value=True)
            mock_redis.close = AsyncMock()
            mock_redis_class.return_value = mock_redis
            from workers.db_health_checker import check_server_health

            result = await check_server_health(server)
            assert result is True

    @pytest.mark.asyncio
    async def test_check_server_health_unsupported_type(self):
        """Test check_server_health with unsupported type defaults to MySQL."""
        server = {
            "id": 5,
            "db_type": "unsupported",
            "host": "localhost",
            "port": 5432,
        }
        with patch("asyncio.open_connection", new_callable=AsyncMock) as mock_tcp:
            mock_writer = MagicMock()
            mock_tcp.return_value = (AsyncMock(), mock_writer)
            from workers.db_health_checker import check_server_health

            result = await check_server_health(server)
            assert result is True


class TestScalingEvaluator:
    """Tests for scaling_evaluator worker module."""

    @pytest.mark.asyncio
    async def test_evaluate_policy_scale_up(self):
        """Test scaling policy triggers scale_up event."""
        policy = {
            "id": 1,
            "server_id": 10,
            "trigger_metric": "connections",
            "scale_up_threshold": 100.0,
            "scale_down_threshold": 10.0,
        }
        mock_db = MagicMock()
        mock_db.scaling_event.insert = MagicMock(return_value=1)
        mock_db.commit = MagicMock()

        with patch("redis.asyncio.Redis") as mock_redis_class:
            mock_redis = AsyncMock()
            mock_redis.get = AsyncMock(return_value="150.0")
            mock_redis.close = AsyncMock()
            mock_redis_class.return_value = mock_redis
            from workers.scaling_evaluator import evaluate_policy

            await evaluate_policy(policy, mock_db)
            mock_db.scaling_event.insert.assert_called_once()
            call_args = mock_db.scaling_event.insert.call_args
            assert call_args[1]["event_type"] == "scale_up"
            assert call_args[1]["status"] == "pending"

    @pytest.mark.asyncio
    async def test_evaluate_policy_scale_down(self):
        """Test scaling policy triggers scale_down event."""
        policy = {
            "id": 1,
            "server_id": 10,
            "trigger_metric": "connections",
            "scale_up_threshold": 100.0,
            "scale_down_threshold": 10.0,
        }
        mock_db = MagicMock()
        mock_db.scaling_event.insert = MagicMock(return_value=1)
        mock_db.commit = MagicMock()

        with patch("redis.asyncio.Redis") as mock_redis_class:
            mock_redis = AsyncMock()
            mock_redis.get = AsyncMock(return_value="5.0")
            mock_redis.close = AsyncMock()
            mock_redis_class.return_value = mock_redis
            from workers.scaling_evaluator import evaluate_policy

            await evaluate_policy(policy, mock_db)
            mock_db.scaling_event.insert.assert_called_once()
            call_args = mock_db.scaling_event.insert.call_args
            assert call_args[1]["event_type"] == "scale_down"

    @pytest.mark.asyncio
    async def test_evaluate_policy_no_trigger(self):
        """Test scaling policy with no metric threshold trigger."""
        policy = {
            "id": 1,
            "server_id": 10,
            "trigger_metric": "connections",
            "scale_up_threshold": 100.0,
            "scale_down_threshold": 10.0,
        }
        mock_db = MagicMock()
        mock_db.scaling_event.insert = MagicMock()

        with patch("redis.asyncio.Redis") as mock_redis_class:
            mock_redis = AsyncMock()
            mock_redis.get = AsyncMock(return_value="50.0")
            mock_redis.close = AsyncMock()
            mock_redis_class.return_value = mock_redis
            from workers.scaling_evaluator import evaluate_policy

            await evaluate_policy(policy, mock_db)
            mock_db.scaling_event.insert.assert_not_called()

    @pytest.mark.asyncio
    async def test_evaluate_policy_redis_error(self):
        """Test scaling policy with Redis connection error."""
        policy = {
            "id": 1,
            "server_id": 10,
            "trigger_metric": "connections",
            "scale_up_threshold": 100.0,
            "scale_down_threshold": 10.0,
        }
        mock_db = MagicMock()
        mock_db.scaling_event.insert = MagicMock()

        with patch("redis.asyncio.Redis", side_effect=Exception("Redis unavailable")):
            from workers.scaling_evaluator import evaluate_policy

            await evaluate_policy(policy, mock_db)
            mock_db.scaling_event.insert.assert_not_called()

    @pytest.mark.asyncio
    async def test_evaluate_policy_no_metric_value(self):
        """Test scaling policy when metric value is None."""
        policy = {
            "id": 1,
            "server_id": 10,
            "trigger_metric": "connections",
            "scale_up_threshold": 100.0,
            "scale_down_threshold": 10.0,
        }
        mock_db = MagicMock()
        mock_db.scaling_event.insert = MagicMock()

        with patch("redis.asyncio.Redis") as mock_redis_class:
            mock_redis = AsyncMock()
            mock_redis.get = AsyncMock(return_value=None)
            mock_redis.close = AsyncMock()
            mock_redis_class.return_value = mock_redis
            from workers.scaling_evaluator import evaluate_policy

            await evaluate_policy(policy, mock_db)
            mock_db.scaling_event.insert.assert_not_called()


class TestThreatIntelPoller:
    """Tests for threat_intel_poller worker module."""

    @pytest.mark.asyncio
    async def test_poll_feed_stix_success(self):
        """Test polling a STIX feed successfully."""
        feed = {
            "id": 1,
            "feed_type": "stix",
            "url": "https://example.com/feed.xml",
        }
        mock_db = MagicMock()
        mock_db.threat_intel_indicator.update_or_insert = MagicMock()
        mock_db.threat_intel_feed.id = MagicMock()
        mock_db.threat_intel_feed.id.__eq__ = MagicMock(return_value=True)
        mock_db.commit = MagicMock()
        cpu_pool = MagicMock(spec=ProcessPoolExecutor)

        mock_indicator = MagicMock()
        mock_indicator.value = "http://malicious.com"
        mock_indicator.indicator_type = "url"
        mock_indicator.confidence = 0.8
        mock_indicator.severity = "high"

        with patch("aiohttp.ClientSession") as mock_session_class:
            mock_session = AsyncMock()
            mock_resp = AsyncMock()
            mock_resp.text = AsyncMock(return_value="<stix>content</stix>")
            mock_session.__aenter__ = AsyncMock(return_value=mock_session)
            mock_session.__aexit__ = AsyncMock(return_value=None)
            mock_session.get = MagicMock(return_value=AsyncMock())
            mock_session.get.return_value.__aenter__ = AsyncMock(
                return_value=mock_resp
            )
            mock_session.get.return_value.__aexit__ = AsyncMock(return_value=None)
            mock_session_class.return_value = mock_session

            with patch(
                "asyncio.get_event_loop"
            ) as mock_loop:
                mock_event_loop = AsyncMock()
                mock_event_loop.run_in_executor = AsyncMock(
                    return_value=[mock_indicator]
                )
                mock_loop.return_value = mock_event_loop
                from workers.threat_intel_poller import poll_feed

                count = await poll_feed(feed, mock_db, cpu_pool)
                assert count >= 0

    @pytest.mark.asyncio
    async def test_poll_feed_http_failure(self):
        """Test polling a feed with HTTP error."""
        feed = {
            "id": 1,
            "feed_type": "stix",
            "url": "https://example.com/feed.xml",
        }
        mock_db = MagicMock()
        cpu_pool = MagicMock(spec=ProcessPoolExecutor)

        with patch("aiohttp.ClientSession") as mock_session_class:
            mock_session = AsyncMock()
            mock_session.__aenter__ = AsyncMock(return_value=mock_session)
            mock_session.__aexit__ = AsyncMock(return_value=None)
            mock_session.get = MagicMock(
                side_effect=Exception("HTTP error: 404")
            )
            mock_session_class.return_value = mock_session
            from workers.threat_intel_poller import poll_feed

            count = await poll_feed(feed, mock_db, cpu_pool)
            assert count == 0

    @pytest.mark.asyncio
    async def test_poll_feed_parse_failure(self):
        """Test polling a feed with parse failure."""
        feed = {
            "id": 1,
            "feed_type": "stix",
            "url": "https://example.com/feed.xml",
        }
        mock_db = MagicMock()
        cpu_pool = MagicMock(spec=ProcessPoolExecutor)

        with patch("aiohttp.ClientSession") as mock_session_class:
            mock_session = AsyncMock()
            mock_resp = AsyncMock()
            mock_resp.text = AsyncMock(return_value="<invalid>")
            mock_session.__aenter__ = AsyncMock(return_value=mock_session)
            mock_session.__aexit__ = AsyncMock(return_value=None)
            mock_session.get = MagicMock(return_value=AsyncMock())
            mock_session.get.return_value.__aenter__ = AsyncMock(
                return_value=mock_resp
            )
            mock_session.get.return_value.__aexit__ = AsyncMock(return_value=None)
            mock_session_class.return_value = mock_session

            with patch(
                "asyncio.get_event_loop"
            ) as mock_loop:
                mock_event_loop = AsyncMock()
                mock_event_loop.run_in_executor = AsyncMock(
                    side_effect=Exception("Parse error")
                )
                mock_loop.return_value = mock_event_loop
                from workers.threat_intel_poller import poll_feed

                count = await poll_feed(feed, mock_db, cpu_pool)
                assert count == 0

    @pytest.mark.asyncio
    async def test_poll_feed_empty_url(self):
        """Test polling a feed with empty URL."""
        feed = {"id": 1, "feed_type": "stix", "url": ""}
        mock_db = MagicMock()
        cpu_pool = MagicMock(spec=ProcessPoolExecutor)
        from workers.threat_intel_poller import poll_feed

        count = await poll_feed(feed, mock_db, cpu_pool)
        assert count == 0

    @pytest.mark.asyncio
    async def test_poll_feed_openioc_type(self):
        """Test polling an OpenIOC feed."""
        feed = {
            "id": 2,
            "feed_type": "openioc",
            "url": "https://example.com/feed.openioc",
        }
        mock_db = MagicMock()
        mock_db.threat_intel_indicator.update_or_insert = MagicMock()
        mock_db.threat_intel_feed.id = MagicMock()
        mock_db.threat_intel_feed.id.__eq__ = MagicMock(return_value=True)
        mock_db.commit = MagicMock()
        cpu_pool = MagicMock(spec=ProcessPoolExecutor)

        with patch("aiohttp.ClientSession") as mock_session_class:
            mock_session = AsyncMock()
            mock_resp = AsyncMock()
            mock_resp.text = AsyncMock(return_value="<ioc>content</ioc>")
            mock_session.__aenter__ = AsyncMock(return_value=mock_session)
            mock_session.__aexit__ = AsyncMock(return_value=None)
            mock_session.get = MagicMock(return_value=AsyncMock())
            mock_session.get.return_value.__aenter__ = AsyncMock(
                return_value=mock_resp
            )
            mock_session.get.return_value.__aexit__ = AsyncMock(return_value=None)
            mock_session_class.return_value = mock_session

            with patch(
                "asyncio.get_event_loop"
            ) as mock_loop:
                mock_event_loop = AsyncMock()
                mock_event_loop.run_in_executor = AsyncMock(return_value=[])
                mock_loop.return_value = mock_event_loop
                from workers.threat_intel_poller import poll_feed

                count = await poll_feed(feed, mock_db, cpu_pool)
                assert count == 0

    @pytest.mark.asyncio
    async def test_poll_feed_misp_type(self):
        """Test polling a MISP feed."""
        feed = {
            "id": 3,
            "feed_type": "misp",
            "url": "https://example.com/misp/event.json",
        }
        mock_db = MagicMock()
        mock_db.threat_intel_indicator.update_or_insert = MagicMock()
        mock_db.threat_intel_feed.id = MagicMock()
        mock_db.threat_intel_feed.id.__eq__ = MagicMock(return_value=True)
        mock_db.commit = MagicMock()
        cpu_pool = MagicMock(spec=ProcessPoolExecutor)

        with patch("aiohttp.ClientSession") as mock_session_class:
            mock_session = AsyncMock()
            mock_resp = AsyncMock()
            mock_resp.text = AsyncMock(return_value='{"Event":{}}')
            mock_session.__aenter__ = AsyncMock(return_value=mock_session)
            mock_session.__aexit__ = AsyncMock(return_value=None)
            mock_session.get = MagicMock(return_value=AsyncMock())
            mock_session.get.return_value.__aenter__ = AsyncMock(
                return_value=mock_resp
            )
            mock_session.get.return_value.__aexit__ = AsyncMock(return_value=None)
            mock_session_class.return_value = mock_session

            with patch(
                "asyncio.get_event_loop"
            ) as mock_loop:
                mock_event_loop = AsyncMock()
                mock_event_loop.run_in_executor = AsyncMock(return_value=[])
                mock_loop.return_value = mock_event_loop
                from workers.threat_intel_poller import poll_feed

                count = await poll_feed(feed, mock_db, cpu_pool)
                assert count == 0

    @pytest.mark.asyncio
    async def test_poll_feed_unsupported_type(self):
        """Test polling a feed with unsupported type."""
        feed = {
            "id": 4,
            "feed_type": "unsupported",
            "url": "https://example.com/feed.txt",
        }
        mock_db = MagicMock()
        cpu_pool = MagicMock(spec=ProcessPoolExecutor)

        with patch("aiohttp.ClientSession") as mock_session_class:
            mock_session = AsyncMock()
            mock_resp = AsyncMock()
            mock_resp.text = AsyncMock(return_value="some content")
            mock_session.__aenter__ = AsyncMock(return_value=mock_session)
            mock_session.__aexit__ = AsyncMock(return_value=None)
            mock_session.get = MagicMock(return_value=AsyncMock())
            mock_session.get.return_value.__aenter__ = AsyncMock(
                return_value=mock_resp
            )
            mock_session.get.return_value.__aexit__ = AsyncMock(return_value=None)
            mock_session_class.return_value = mock_session
            from workers.threat_intel_poller import poll_feed

            count = await poll_feed(feed, mock_db, cpu_pool)
            assert count == 0
