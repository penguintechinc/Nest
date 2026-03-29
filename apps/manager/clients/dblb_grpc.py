"""gRPC client for MarchProxy DBLB ModuleService."""
import asyncio
import logging
import os
from dataclasses import dataclass

logger = logging.getLogger(__name__)

DBLB_GRPC_HOST = os.environ.get("DBLB_GRPC_HOST", "marchproxy-dblb")
DBLB_GRPC_PORT = int(os.environ.get("DBLB_GRPC_PORT", "50052"))


@dataclass(slots=True)
class DblbStatus:
    status: str
    version: str
    uptime_seconds: int
    active_connections: int
    backend_count: int


@dataclass(slots=True)
class DblbReloadResult:
    success: bool
    message: str
    routes_loaded: int


class DblbGrpcClient:
    """Async gRPC client for the DBLB ModuleService."""

    def __init__(self, host: str = DBLB_GRPC_HOST, port: int = DBLB_GRPC_PORT):
        self.host = host
        self.port = port
        self._channel = None
        self._stub = None

    async def _ensure_connected(self):
        if self._stub is None:
            try:
                import grpc
                from proto import dblb_pb2_grpc
                self._channel = grpc.aio.insecure_channel(f"{self.host}:{self.port}")
                self._stub = dblb_pb2_grpc.ModuleServiceStub(self._channel)
            except Exception as e:
                logger.warning(f"DBLB gRPC connection failed: {e}. Running in degraded mode.")

    async def reload(self, force: bool = False) -> DblbReloadResult:
        """Trigger DBLB to reload routes from Redis."""
        await self._ensure_connected()
        if self._stub is None:
            logger.warning("DBLB gRPC unavailable, reload skipped")
            return DblbReloadResult(success=False, message="gRPC unavailable", routes_loaded=0)
        try:
            from proto import dblb_pb2
            response = await self._stub.Reload(dblb_pb2.ReloadRequest(force=force))
            return DblbReloadResult(
                success=response.success,
                message=response.message,
                routes_loaded=response.routes_loaded,
            )
        except Exception as e:
            logger.error(f"DBLB Reload failed: {e}")
            return DblbReloadResult(success=False, message=str(e), routes_loaded=0)

    async def get_status(self) -> DblbStatus:
        """Get DBLB operational status."""
        await self._ensure_connected()
        if self._stub is None:
            return DblbStatus(status="unavailable", version="unknown", uptime_seconds=0,
                              active_connections=0, backend_count=0)
        try:
            from proto import dblb_pb2
            r = await self._stub.GetStatus(dblb_pb2.StatusRequest())
            return DblbStatus(
                status=r.status, version=r.version, uptime_seconds=r.uptime_seconds,
                active_connections=r.active_connections, backend_count=r.backend_count,
            )
        except Exception as e:
            logger.error(f"DBLB GetStatus failed: {e}")
            return DblbStatus(status="error", version="unknown", uptime_seconds=0,
                              active_connections=0, backend_count=0)

    async def health_check(self) -> bool:
        """Returns True if DBLB is healthy."""
        await self._ensure_connected()
        if self._stub is None:
            return False
        try:
            from proto import dblb_pb2
            r = await self._stub.HealthCheck(dblb_pb2.HealthCheckRequest())
            return r.healthy
        except Exception:
            return False

    async def close(self):
        if self._channel:
            await self._channel.close()


# Module-level singleton
_client: DblbGrpcClient | None = None


def get_dblb_client() -> DblbGrpcClient:
    global _client
    if _client is None:
        _client = DblbGrpcClient()
    return _client
