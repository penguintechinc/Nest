"""Worker test isolation: clear all worker stubs before loading worker modules."""
import os
import sys
from pathlib import Path

# Ensure app root is in sys.path for worker imports
app_root = Path(__file__).parent.parent
if str(app_root) not in sys.path:
    sys.path.insert(0, str(app_root))

# Remove stubs injected by route tests so real modules are imported
_WORKER_STUBS = [
    "workers",
    "workers.db_health_checker",
    "workers.scaling_evaluator",
    "workers.threat_intel_poller",
    "workers.backup_scheduler",
    "workers.cert_rotation",
    "workers.stats_collector",
    "workers.user_sync",
    "models",
]
for _m in _WORKER_STUBS:
    sys.modules.pop(_m, None)

# Set environment defaults
os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("REDIS_HOST", "localhost")
os.environ.setdefault("REDIS_PORT", "6379")
