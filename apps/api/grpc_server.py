"""gRPC server stub for Nest API.

Runs health checks on port 50051 in background.
"""
import asyncio
import logging

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc
from grpc_reflection.v1alpha import reflection

logger = logging.getLogger(__name__)


async def start_grpc_server():
    """Start gRPC health server on :50051."""
    server = grpc.aio.server()

    # Register health service
    health_servicer = health.HealthServicer()
    health_servicer.set("nest.api.Nest", health_pb2.HealthCheckResponse.SERVING)
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)

    # Register reflection for gRPC tools
    reflection.enable_server_reflection(
        ("nest.api.Nest", "grpc.health.v1.Health"),
        server,
    )

    # Add port and start
    server.add_insecure_port("[::]:50051")
    await server.start()
    logger.info("gRPC health server started on :50051")

    try:
        await server.wait_for_termination()
    except asyncio.CancelledError:
        await server.stop(0)


def start_grpc_in_background():
    """Start gRPC server in background as asyncio task."""
    asyncio.create_task(start_grpc_server())
