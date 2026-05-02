"""TCP probe utilities for introspection."""
import asyncio
import re
from typing import Optional, Tuple


def extract_host_port(
    conn_str: str, default_port: int = 5432
) -> Optional[Tuple[str, int]]:
    """Extract host:port from a connection string.

    Handles formats like:
    - postgresql://host:5432/db
    - host:5432
    - host (uses default_port)
    """
    if not conn_str:
        return None

    # Try postgres:// URI format
    if "://" in conn_str:
        match = re.search(r"://([^/:]+):?(\d+)?", conn_str)
        if match:
            host = match.group(1)
            port = int(match.group(2)) if match.group(2) else default_port
            return (host, port)

    # Try host:port format
    if ":" in conn_str:
        parts = conn_str.rsplit(":", 1)
        try:
            port = int(parts[1])
            return (parts[0], port)
        except ValueError:
            pass

    # Just host, use default port
    return (conn_str, default_port)


async def tcp_ping(
    host: str, port: int, timeout: float = 5.0
) -> Tuple[bool, int, str]:
    """Probe TCP connection to host:port.

    Returns: (reachable, latency_ms, message)
    """
    import time

    start = time.time()
    try:
        await asyncio.wait_for(
            asyncio.open_connection(host, port),
            timeout=timeout,
        )
        latency_ms = int((time.time() - start) * 1000)
        return True, latency_ms, "tcp connection successful"
    except asyncio.TimeoutError:
        return False, int(timeout * 1000), f"connection timeout after {timeout}s"
    except Exception as e:
        latency_ms = int((time.time() - start) * 1000)
        return False, latency_ms, str(e)
