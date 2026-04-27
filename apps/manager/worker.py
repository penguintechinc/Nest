"""Background worker for processing long-running operations."""

import asyncio
from datetime import datetime, timezone

from store.store import OperationStore


async def run(store: OperationStore) -> None:
    """Main worker loop.

    Polls the operation queue every 2 seconds and advances operation states:
    - pending → running (immediately)
    - running → succeeded (after ~3 seconds)
    """
    try:
        while True:
            await asyncio.sleep(2)

            # Get all pending and running operations
            operations = await store.list_pending_or_running()

            now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
            now_dt = datetime.now(timezone.utc)

            for op in operations:
                # Transition pending → running
                if op.phase == "pending":
                    op.phase = "running"
                    op.message = "Operation in progress"
                    op.updated_at = now
                    await store.update_operation(op)

                # Transition running → succeeded (after ~3 seconds)
                elif op.phase == "running":
                    updated_dt = datetime.fromisoformat(
                        op.updated_at.replace("Z", "+00:00")
                    )
                    elapsed = (now_dt - updated_dt).total_seconds()

                    if elapsed >= 3:
                        op.phase = "succeeded"
                        op.message = "Operation completed"
                        op.progress = 100
                        op.updated_at = now
                        await store.update_operation(op)

    except asyncio.CancelledError:
        pass
