"""gRPC health check server."""

import asyncio

from grpc_health.v1 import health, health_pb2_grpc
import grpc


async def serve(port: int = 50052) -> None:
    """Start gRPC health check server on the specified port."""
    try:
        server = grpc.aio.server()
        health_pb2_grpc.add_HealthServicer_to_server(
            health.HealthServicer(), server
        )

        server.add_insecure_port(f"[::]:{port}")
        await server.start()

        # Keep the server running
        await server.wait_for_termination()

    except asyncio.CancelledError:
        pass
