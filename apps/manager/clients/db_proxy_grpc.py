"""gRPC client for nest DB Proxy ConfigService."""
import json
import logging
import os
from dataclasses import dataclass

logger = logging.getLogger(__name__)

DB_PROXY_GRPC_HOST = os.environ.get("DB_PROXY_GRPC_HOST", "nest-db-proxy")
DB_PROXY_GRPC_PORT = int(os.environ.get("DB_PROXY_GRPC_PORT", "50051"))


@dataclass(slots=True)
class DbProxyStatus:
    status: str
    active_connections: int
    total_connections: int
    queries_processed: int
    queries_blocked: int
    uptime_seconds: str


class DbProxyGrpcClient:
    """Async gRPC client for the nest DB Proxy ConfigService."""

    def __init__(self, host: str = DB_PROXY_GRPC_HOST, port: int = DB_PROXY_GRPC_PORT):
        self.host = host
        self.port = port
        self._channel = None
        self._stub = None

    async def _ensure_connected(self):
        if self._stub is None:
            try:
                import grpc
                from proto import config_pb2_grpc
                self._channel = grpc.aio.insecure_channel(f"{self.host}:{self.port}")
                self._stub = config_pb2_grpc.ConfigServiceStub(self._channel)
            except Exception as e:
                logger.warning(f"DB Proxy gRPC connection failed: {e}. Running in degraded mode.")

    async def reload(self) -> bool:
        """Trigger DB Proxy to reload routes from Redis."""
        await self._ensure_connected()
        if self._stub is None:
            logger.warning("DB Proxy gRPC unavailable, reload skipped")
            return False
        try:
            from proto import config_pb2
            response = await self._stub.ReloadConfig(
                config_pb2.ReloadConfigRequest(api_version="v1")
            )
            success = response.status == "success"
            if not success:
                logger.error(f"DB Proxy ReloadConfig failed: {response.error_message}")
            return success
        except Exception as e:
            logger.error(f"DB Proxy ReloadConfig failed: {e}")
            return False

    async def get_status(self) -> DbProxyStatus:
        """Get DB Proxy operational status."""
        await self._ensure_connected()
        if self._stub is None:
            return DbProxyStatus(
                status="unavailable", active_connections=0, total_connections=0,
                queries_processed=0, queries_blocked=0, uptime_seconds="0"
            )
        try:
            from proto import config_pb2
            r = await self._stub.GetStatus(config_pb2.GetStatusRequest(api_version="v1"))
            return DbProxyStatus(
                status=r.status, active_connections=r.active_connections,
                total_connections=r.total_connections, queries_processed=r.queries_processed,
                queries_blocked=r.queries_blocked, uptime_seconds=r.uptime_seconds,
            )
        except Exception as e:
            logger.error(f"DB Proxy GetStatus failed: {e}")
            return DbProxyStatus(
                status="error", active_connections=0, total_connections=0,
                queries_processed=0, queries_blocked=0, uptime_seconds="0"
            )

    async def health_check(self) -> bool:
        """Returns True if DB Proxy is healthy via gRPC health check."""
        await self._ensure_connected()
        if self._stub is None:
            return False
        try:
            status = await self.get_status()
            return status.status == "healthy"
        except Exception:
            return False

    async def get_blocking_config(self) -> dict:
        """Get security/blocking configuration from DB Proxy."""
        await self._ensure_connected()
        if self._stub is None:
            logger.warning("DB Proxy gRPC unavailable, returning empty config")
            return {"blocked_resources": [], "allowed_resources": [], "enable_injection_check": False}
        try:
            from proto import config_pb2
            response = await self._stub.GetConfig(
                config_pb2.GetConfigRequest(api_version="v1", config_key="security")
            )
            if response.status != "success":
                logger.error(f"DB Proxy GetConfig failed: {response.error_message}")
                return {"blocked_resources": [], "allowed_resources": [], "enable_injection_check": False}
            return json.loads(response.config_data.decode("utf-8"))
        except Exception as e:
            logger.error(f"DB Proxy GetConfig failed: {e}")
            return {"blocked_resources": [], "allowed_resources": [], "enable_injection_check": False}

    async def set_blocking_config(self, config: dict) -> bool:
        """Set security/blocking configuration on DB Proxy."""
        await self._ensure_connected()
        if self._stub is None:
            logger.warning("DB Proxy gRPC unavailable, config not set")
            return False
        try:
            from proto import config_pb2
            config_data = json.dumps(config).encode("utf-8")
            response = await self._stub.SetConfig(
                config_pb2.SetConfigRequest(
                    api_version="v1", config_key="security", config_data=config_data
                )
            )
            success = response.status == "success"
            if not success:
                logger.error(f"DB Proxy SetConfig failed: {response.error_message}")
            return success
        except Exception as e:
            logger.error(f"DB Proxy SetConfig failed: {e}")
            return False

    async def close(self):
        if self._channel:
            await self._channel.close()


# Module-level singleton
_client: DbProxyGrpcClient | None = None


def get_db_proxy_client() -> DbProxyGrpcClient:
    global _client
    if _client is None:
        _client = DbProxyGrpcClient()
    return _client
