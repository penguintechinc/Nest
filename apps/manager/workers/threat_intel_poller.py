"""
Threat Intelligence Poller Worker.
Polls STIX/TAXII/MISP/OpenIOC feeds and stores indicators in DB + Redis.
Runs as an async infinite loop; CPU-bound parsing uses ProcessPoolExecutor.
"""
import asyncio
import logging
import os
from concurrent.futures import ProcessPoolExecutor
from datetime import datetime, timezone

logger = logging.getLogger(__name__)
POLL_INTERVAL = int(os.environ.get("THREAT_INTEL_POLL_INTERVAL", "300"))

async def poll_feed(feed: dict, db, cpu_pool: ProcessPoolExecutor) -> int:
    """Poll a single threat intel feed and store indicators. Returns count stored."""
    import aiohttp
    from utils.threat_intel_parsers import parse_stix_indicators, parse_openioc_indicators, parse_misp_event

    feed_id = feed["id"]
    feed_type = feed.get("feed_type", "stix")
    url = feed.get("url", "")

    if not url:
        return 0

    try:
        async with aiohttp.ClientSession() as session:
            async with session.get(url, timeout=aiohttp.ClientTimeout(total=30)) as resp:
                content = await resp.text()
    except Exception as e:
        logger.warning(f"Failed to fetch feed {feed_id}: {e}")
        return 0

    loop = asyncio.get_event_loop()
    try:
        if feed_type == "stix":
            indicators = await loop.run_in_executor(cpu_pool, parse_stix_indicators, content)
        elif feed_type == "openioc":
            indicators = await loop.run_in_executor(cpu_pool, parse_openioc_indicators, content)
        elif feed_type == "misp":
            indicators = await loop.run_in_executor(cpu_pool, parse_misp_event, content)
        else:
            return 0
    except Exception as e:
        logger.error(f"Failed to parse feed {feed_id} ({feed_type}): {e}")
        return 0

    count = 0
    now = datetime.now(timezone.utc)
    for ind in indicators:
        try:
            await asyncio.to_thread(
                lambda i=ind: db.threat_intel_indicator.update_or_insert(
                    (db.threat_intel_indicator.feed_id == feed_id) &
                    (db.threat_intel_indicator.value == i.value),
                    feed_id=feed_id,
                    indicator_type=i.indicator_type,
                    value=i.value,
                    confidence=i.confidence,
                    severity=i.severity,
                    created_at=now,
                )
            )
            count += 1
        except Exception as e:
            logger.debug(f"Failed to store indicator: {e}")

    await asyncio.to_thread(lambda: db(db.threat_intel_feed.id == feed_id).update(
        last_polled_at=now
    ))
    await asyncio.to_thread(db.commit)
    return count

async def threat_intel_poller_loop(db, cpu_pool: ProcessPoolExecutor) -> None:
    """Main poller loop - runs indefinitely."""
    logger.info("Threat Intel Poller started")
    while True:
        try:
            feeds = await asyncio.to_thread(
                lambda: db(
                    (db.threat_intel_feed.active == True)
                ).select(db.threat_intel_feed.ALL).as_list()
            )
            for feed in feeds:
                count = await poll_feed(feed, db, cpu_pool)
                if count:
                    logger.info(f"Feed {feed['id']}: stored {count} indicators")

            # Sync to Redis after polling all feeds
            from utils.redis_sync import sync_threat_intel_to_redis
            await sync_threat_intel_to_redis(db)

        except Exception as e:
            logger.error(f"Threat intel poller error: {e}", exc_info=True)

        await asyncio.sleep(POLL_INTERVAL)
