"""
Scaling Evaluator Worker.
Reads health metrics, evaluates scaling policies, triggers cloud scale operations.
"""
import asyncio
import logging
import os
from datetime import datetime, timezone

logger = logging.getLogger(__name__)
SCALING_EVAL_INTERVAL = int(os.environ.get("SCALING_EVAL_INTERVAL", "120"))

async def evaluate_policy(policy: dict, db) -> None:
    """Evaluate a single scaling policy and trigger if threshold met."""
    server_id = policy.get("server_id")
    metric = policy.get("trigger_metric", "connections")
    scale_up = policy.get("scale_up_threshold")
    scale_down = policy.get("scale_down_threshold")

    # Get current metric from Redis or DB
    current_value = None
    try:
        import redis.asyncio as aioredis
        r = aioredis.Redis(
            host=os.environ.get("REDIS_HOST", "redis"),
            port=int(os.environ.get("REDIS_PORT", "6379")),
            decode_responses=True
        )
        val = await r.get(f"nest:server:{server_id}:metric:{metric}")
        await r.close()
        if val is not None:
            current_value = float(val)
    except Exception:
        pass

    if current_value is None:
        return

    now = datetime.now(timezone.utc)
    event_type = None
    if scale_up is not None and current_value >= scale_up:
        event_type = "scale_up"
    elif scale_down is not None and current_value <= scale_down:
        event_type = "scale_down"

    if event_type:
        try:
            await asyncio.to_thread(
                lambda: db.scaling_event.insert(
                    policy_id=policy["id"],
                    server_id=server_id,
                    event_type=event_type,
                    status="pending",
                    triggered_at=now,
                )
            )
            await asyncio.to_thread(db.commit)
            logger.info(f"Scaling event triggered: server={server_id} type={event_type} metric={current_value}")
        except Exception as e:
            logger.error(f"Failed to record scaling event: {e}")

async def scaling_evaluator_loop(db) -> None:
    """Main scaling evaluator loop - runs indefinitely."""
    logger.info("Scaling Evaluator started")
    while True:
        try:
            policies = await asyncio.to_thread(
                lambda: db(db.scaling_policy.active == True).select(
                    db.scaling_policy.ALL
                ).as_list()
            )
            for policy in policies:
                await evaluate_policy(policy, db)
        except Exception as e:
            logger.error(f"Scaling evaluator error: {e}", exc_info=True)

        await asyncio.sleep(SCALING_EVAL_INTERVAL)
