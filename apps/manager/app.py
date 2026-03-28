"""
Manager Service - Main Quart Application

NEST Manager orchestrates infrastructure operations including:
- Kubernetes cluster management
- Resource provisioning and lifecycle
- Certificate generation and rotation
- Backup and disaster recovery
- Infrastructure monitoring
"""

import os
import logging
from concurrent.futures import ProcessPoolExecutor
from quart import Quart, jsonify
from quart_cors import cors
from prometheus_client import generate_latest, CollectorRegistry, Counter, Histogram
import time

# Initialize Quart application
app = Quart(__name__)
app = cors(app, allow_origin="*")

# Configure environment variables
app.config['DEBUG'] = os.getenv('DEBUG', 'False').lower() == 'true'
app.config['HOST'] = os.getenv('HOST', '0.0.0.0')
app.config['PORT'] = int(os.getenv('PORT', '5000'))
app.config['DB_TYPE'] = os.getenv('DB_TYPE', 'postgresql')
app.config['DB_HOST'] = os.getenv('DB_HOST', 'localhost')
app.config['DB_PORT'] = os.getenv('DB_PORT', '5432')
app.config['DB_NAME'] = os.getenv('DB_NAME', 'nest')
app.config['DB_USER'] = os.getenv('DB_USER', 'nest')

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Initialize Prometheus metrics
registry = CollectorRegistry()
http_requests_total = Counter(
    'http_requests_total',
    'Total HTTP requests',
    ['method', 'endpoint', 'status'],
    registry=registry
)
http_request_duration = Histogram(
    'http_request_duration_seconds',
    'HTTP request duration in seconds',
    ['method', 'endpoint'],
    registry=registry
)

# Register penguin-dal async DB lifecycle hooks
# Use get_db() from penguin_dal.quart_ext in request handlers
try:
    from penguin_dal.quart_ext import init_dal
    from models import db
    init_dal(app)
    logger.info("penguin-dal initialized successfully")
except Exception as e:
    logger.error(f"Failed to initialize penguin-dal: {e}")
    db = None  # type: ignore[assignment]
    # Continue with app startup even if models fail

# ---------------------------------------------------------------------------
# Shared resources (initialized in before_serving, cleaned up in after_serving)
# ---------------------------------------------------------------------------

# ProcessPoolExecutor for CPU-bound tasks (SQL validation, etc.)
cpu_pool: ProcessPoolExecutor = ProcessPoolExecutor(max_workers=4)
app.cpu_pool = cpu_pool  # type: ignore[attr-defined]

# DblbGrpcClient singleton
try:
    from clients.dblb_grpc import get_dblb_client, init_dblb_client
    _dblb_client = init_dblb_client()
    logger.info("DblbGrpcClient initialized")
except Exception as exc:
    logger.warning("DblbGrpcClient initialization failed (non-fatal): %s", exc)
    _dblb_client = None

# ---------------------------------------------------------------------------
# Register all blueprints
# ---------------------------------------------------------------------------

from routes.database_servers import servers_bp
from routes.permissions import permissions_bp
from routes.user_profiles import profiles_bp
from routes.temporary_access import temp_access_bp
from routes.security_rules import security_rules_bp
from routes.managed_databases import databases_bp
from routes.sql_files import sql_files_bp
from routes.blocked_databases import blocked_bp
from routes.threat_intel import threat_intel_bp
from routes.cloud import cloud_bp
from routes.scaling import scaling_bp
from routes.license import license_bp
from routes.sync import sync_bp
from routes.audit import audit_bp
from routes.stats import stats_bp
from routes.auth import auth_bp
from routes.teams import teams_bp
from routes.analytics import analytics_bp

app.register_blueprint(servers_bp)
app.register_blueprint(permissions_bp)
app.register_blueprint(profiles_bp)
app.register_blueprint(temp_access_bp)
app.register_blueprint(security_rules_bp)
app.register_blueprint(databases_bp)
app.register_blueprint(sql_files_bp)
app.register_blueprint(blocked_bp)
app.register_blueprint(threat_intel_bp)
app.register_blueprint(cloud_bp)
app.register_blueprint(scaling_bp)
app.register_blueprint(license_bp)
app.register_blueprint(sync_bp)
app.register_blueprint(audit_bp)
app.register_blueprint(stats_bp)
app.register_blueprint(auth_bp)
app.register_blueprint(teams_bp)
app.register_blueprint(analytics_bp)


# ---------------------------------------------------------------------------
# Lifecycle hooks
# ---------------------------------------------------------------------------

@app.before_serving
async def startup():
    """Start background worker tasks."""
    import asyncio
    from workers.threat_intel_poller import threat_intel_poller_loop
    from workers.db_health_checker import db_health_checker_loop
    from workers.scaling_evaluator import scaling_evaluator_loop

    loop = asyncio.get_event_loop()
    loop.create_task(threat_intel_poller_loop(db, cpu_pool))
    loop.create_task(db_health_checker_loop(db))
    loop.create_task(scaling_evaluator_loop(db))
    logger.info("Background worker tasks started")


@app.after_serving
async def shutdown():
    """Clean up shared resources on shutdown."""
    try:
        cpu_pool.shutdown(wait=False)
        logger.info("ProcessPoolExecutor shut down")
    except Exception as exc:
        logger.warning("Error shutting down cpu_pool: %s", exc)

    if _dblb_client is not None:
        try:
            _dblb_client.close()
            logger.info("DblbGrpcClient closed")
        except Exception as exc:
            logger.warning("Error closing DblbGrpcClient: %s", exc)


# ---------------------------------------------------------------------------
# Built-in endpoints (unchanged)
# ---------------------------------------------------------------------------

# Health check endpoint
@app.route('/healthz', methods=['GET'])
async def healthz():
    """Health check endpoint for Kubernetes probes"""
    return jsonify({"status": "healthy"}), 200


# Metrics endpoint
@app.route('/metrics', methods=['GET'])
async def metrics():
    """Prometheus metrics endpoint"""
    return generate_latest(registry), 200, {'Content-Type': 'text/plain; charset=utf-8'}


# Middleware to track metrics
@app.before_request
async def before_request():
    """Store request start time for duration tracking"""
    from quart import request as quart_request
    quart_request.start_time = time.time()


@app.after_request
async def after_request(response):
    """Track request metrics after response"""
    from quart import request as quart_request

    if hasattr(quart_request, 'start_time'):
        duration = time.time() - quart_request.start_time
        http_request_duration.labels(
            method=quart_request.method,
            endpoint=quart_request.path
        ).observe(duration)

    http_requests_total.labels(
        method=quart_request.method,
        endpoint=quart_request.path,
        status=response.status_code
    ).inc()

    return response


@app.errorhandler(404)
async def not_found(error):
    """Handle 404 errors"""
    return jsonify({"error": "Not found"}), 404


@app.errorhandler(500)
async def server_error(error):
    """Handle 500 errors"""
    logger.error(f"Server error: {error}")
    return jsonify({"error": "Internal server error"}), 500


if __name__ == '__main__':
    logger.info(f"Starting Manager service on {app.config['HOST']}:{app.config['PORT']}")
    app.run(
        host=app.config['HOST'],
        port=app.config['PORT'],
        debug=app.config['DEBUG']
    )
