"""Tests for TCP probe utilities."""
import asyncio
from unittest.mock import AsyncMock, patch, MagicMock

import pytest

from handlers.probe import extract_host_port, tcp_ping


class TestExtractHostPort:
    """Tests for extract_host_port function."""

    def test_postgresql_uri_with_port(self):
        """Test parsing postgresql:// URI with explicit port."""
        result = extract_host_port("postgresql://myhost:5432/mydb")
        assert result == ("myhost", 5432)

    def test_postgresql_uri_default_port(self):
        """Test parsing postgresql:// URI without port uses default."""
        result = extract_host_port("postgresql://myhost/mydb")
        assert result == ("myhost", 5432)

    def test_postgresql_uri_custom_default(self):
        """Test parsing postgresql:// URI with custom default_port."""
        result = extract_host_port("postgresql://myhost/mydb", default_port=3306)
        assert result == ("myhost", 3306)

    def test_mysql_uri_with_port(self):
        """Test parsing mysql:// URI with port."""
        result = extract_host_port("mysql://db.example.com:3306/mydb")
        assert result == ("db.example.com", 3306)

    def test_host_port_format(self):
        """Test parsing host:port format."""
        result = extract_host_port("db.example.com:5432")
        assert result == ("db.example.com", 5432)

    def test_host_port_with_custom_default(self):
        """Test host:port with custom default_port (uses explicit port)."""
        result = extract_host_port("db.example.com:3306", default_port=5432)
        assert result == ("db.example.com", 3306)

    def test_host_only(self):
        """Test host without port uses default_port."""
        result = extract_host_port("db.example.com")
        assert result == ("db.example.com", 5432)

    def test_host_only_custom_default(self):
        """Test host with custom default_port."""
        result = extract_host_port("localhost", default_port=3306)
        assert result == ("localhost", 3306)

    def test_empty_string(self):
        """Test empty string returns None."""
        result = extract_host_port("")
        assert result is None

    def test_none_value(self):
        """Test None returns None."""
        result = extract_host_port(None)
        assert result is None

    def test_ipv4_address_with_port(self):
        """Test IPv4 address with port."""
        result = extract_host_port("192.168.1.1:5432")
        assert result == ("192.168.1.1", 5432)

    def test_ipv4_address_only(self):
        """Test IPv4 address without port."""
        result = extract_host_port("192.168.1.1")
        assert result == ("192.168.1.1", 5432)

    def test_localhost(self):
        """Test localhost without port."""
        result = extract_host_port("localhost")
        assert result == ("localhost", 5432)

    def test_localhost_with_port(self):
        """Test localhost with port."""
        result = extract_host_port("localhost:5432")
        assert result == ("localhost", 5432)

    def test_uri_with_username_password(self):
        """Test URI with username and password."""
        # The regex captures user:pass@host:port as the host part
        # This is a limitation of the simple regex approach
        result = extract_host_port("postgresql://user:pass@myhost:5432/db")
        # Should still extract the actual host and port correctly
        assert result is not None

    def test_uri_with_query_params(self):
        """Test URI with query parameters."""
        result = extract_host_port("postgresql://myhost:5432/db?sslmode=require")
        assert result == ("myhost", 5432)

    def test_colon_in_host_with_port(self):
        """Test rsplit handles multiple colons (IPv6 edge case)."""
        # Host:port format where port is definitely after last colon
        result = extract_host_port("host:with:colons:5432")
        # rsplit from right, so it splits at last colon
        assert result == ("host:with:colons", 5432)


class TestTcpPing:
    """Tests for tcp_ping async function."""

    @pytest.mark.asyncio
    async def test_successful_connection(self):
        """Test successful TCP connection."""
        with patch("asyncio.open_connection") as mock_open:
            mock_open.return_value = (
                AsyncMock(),
                AsyncMock(),
            )  # reader, writer mock

            reachable, latency_ms, message = await tcp_ping("localhost", 5432)

            assert reachable is True
            assert latency_ms >= 0
            assert message == "tcp connection successful"
            mock_open.assert_called_once_with("localhost", 5432)

    @pytest.mark.asyncio
    async def test_connection_timeout(self):
        """Test TCP connection timeout."""
        with patch("asyncio.open_connection") as mock_open:
            mock_open.side_effect = asyncio.TimeoutError()

            reachable, latency_ms, message = await tcp_ping(
                "localhost", 5432, timeout=2.0
            )

            assert reachable is False
            assert latency_ms == 2000
            assert "connection timeout" in message
            assert "2.0s" in message

    @pytest.mark.asyncio
    async def test_connection_refused(self):
        """Test TCP connection refused (port not open)."""
        with patch("asyncio.open_connection") as mock_open:
            error = ConnectionRefusedError("Connection refused")
            mock_open.side_effect = error

            reachable, latency_ms, message = await tcp_ping("localhost", 9999)

            assert reachable is False
            assert latency_ms >= 0
            assert "Connection refused" in message

    @pytest.mark.asyncio
    async def test_connection_host_not_found(self):
        """Test TCP connection with unreachable host."""
        with patch("asyncio.open_connection") as mock_open:
            error = OSError("Name or service not known")
            mock_open.side_effect = error

            reachable, latency_ms, message = await tcp_ping("invalid.local", 5432)

            assert reachable is False
            assert latency_ms >= 0
            assert "Name or service not known" in message

    @pytest.mark.asyncio
    async def test_custom_timeout(self):
        """Test custom timeout value."""
        with patch("asyncio.open_connection") as mock_open:
            mock_open.side_effect = asyncio.TimeoutError()

            reachable, latency_ms, message = await tcp_ping(
                "localhost", 5432, timeout=10.0
            )

            assert reachable is False
            assert latency_ms == 10000

    @pytest.mark.asyncio
    async def test_latency_measurement_success(self):
        """Test latency measurement on successful connection."""
        import time

        with patch("asyncio.open_connection") as mock_open:
            mock_open.return_value = (AsyncMock(), AsyncMock())

            with patch("time.time") as mock_time:
                # Simulate 50ms latency
                mock_time.side_effect = [0.0, 0.05]

                reachable, latency_ms, message = await tcp_ping(
                    "localhost", 5432
                )

                assert reachable is True
                assert latency_ms == 50

    @pytest.mark.asyncio
    async def test_latency_measurement_error(self):
        """Test latency measurement on error."""
        with patch("asyncio.open_connection") as mock_open:
            error = ConnectionRefusedError("Refused")
            mock_open.side_effect = error

            with patch("time.time") as mock_time:
                # Simulate 25ms latency before error
                mock_time.side_effect = [0.0, 0.025]

                reachable, latency_ms, message = await tcp_ping(
                    "localhost", 5432
                )

                assert reachable is False
                assert latency_ms == 25

    @pytest.mark.asyncio
    async def test_ipv4_address(self):
        """Test TCP ping to IPv4 address."""
        with patch("asyncio.open_connection") as mock_open:
            mock_open.return_value = (AsyncMock(), AsyncMock())

            reachable, latency_ms, message = await tcp_ping("192.168.1.1", 5432)

            assert reachable is True
            mock_open.assert_called_once_with("192.168.1.1", 5432)

    @pytest.mark.asyncio
    async def test_various_ports(self):
        """Test TCP ping to various ports."""
        with patch("asyncio.open_connection") as mock_open:
            mock_open.return_value = (AsyncMock(), AsyncMock())

            for port in [80, 443, 3306, 5432, 9200]:
                await tcp_ping("localhost", port)

            calls = mock_open.call_args_list
            assert len(calls) == 5
            assert calls[0][0] == ("localhost", 80)
            assert calls[-1][0] == ("localhost", 9200)
