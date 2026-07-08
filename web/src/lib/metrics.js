/**
 * Prometheus metrics helper - lightweight stub
 * Provides minimal Prometheus-compatible interface without external dependencies
 * Production deployments can replace with full prom-client implementation
 */

class PrometheusMetrics {
  constructor() {
    this.histogramBuckets = {};
    this.counterValues = {};
  }

  register = {
    contentType: 'text/plain; charset=utf-8',
    metrics: async () => this._formatMetrics(),
  };

  _formatMetrics() {
    let output = '# HELP http_request_duration_seconds Duration of HTTP requests in seconds\n';
    output += '# TYPE http_request_duration_seconds histogram\n';
    output += '# HELP http_requests_total Total number of HTTP requests\n';
    output += '# TYPE http_requests_total counter\n';
    return output;
  }

  Histogram = class {
    constructor(opts) {
      this.name = opts.name;
      this.buckets = {};
    }
    labels(...args) {
      return { observe: () => {} };
    }
  };

  Counter = class {
    constructor(opts) {
      this.name = opts.name;
      this.values = {};
    }
    labels(...args) {
      return { inc: () => {} };
    }
  };
}

export function createPrometheusMetrics() {
  const metrics = new PrometheusMetrics();

  return {
    register: metrics.register,
    httpRequestDuration: new metrics.Histogram({
      name: 'http_request_duration_seconds',
      help: 'Duration of HTTP requests in seconds',
      labelNames: ['method', 'route', 'status_code'],
      buckets: [0.1, 0.5, 1, 2, 5],
    }),
    httpRequestsTotal: new metrics.Counter({
      name: 'http_requests_total',
      help: 'Total number of HTTP requests',
      labelNames: ['method', 'route', 'status_code'],
    }),
  };
}
