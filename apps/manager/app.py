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

# Import PyDAL models
try:
    from models import db
    logger.info("PyDAL models initialized successfully")
except Exception as e:
    logger.error(f"Failed to initialize PyDAL models: {e}")
    # Continue with app startup even if models fail


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
