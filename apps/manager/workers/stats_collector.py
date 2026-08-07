"""Statistics Collector Worker

Periodically collects operational metrics from all managed resources (both
Kubernetes-based and external resources), calculates risk assessments, and
exports metrics to Prometheus for monitoring and alerting.

Features:
- Support for Kubernetes and external resources
- Comprehensive metrics collection (CPU, memory, disk, network, connections)
- Risk level calculation based on operational thresholds
- Prometheus metrics export
- Configurable collection intervals
"""

import os
import time
import logging
from typing import Dict, List, Any, Optional
from dataclasses import dataclass, field
from datetime import datetime
import threading
import json

from prometheus_client import Gauge, Counter

logger = logging.getLogger(__name__)


# Prometheus metrics
RESOURCE_CPU_PERCENT = Gauge(
    'nest_resource_cpu_percent',
    'Resource CPU usage percentage',
    ['resource_id', 'resource_name']
)

RESOURCE_MEMORY_BYTES = Gauge(
    'nest_resource_memory_bytes',
    'Resource memory usage in bytes',
    ['resource_id', 'resource_name']
)

RESOURCE_MEMORY_PERCENT = Gauge(
    'nest_resource_memory_percent',
    'Resource memory usage percentage',
    ['resource_id', 'resource_name']
)

RESOURCE_DISK_USAGE_PERCENT = Gauge(
    'nest_resource_disk_usage_percent',
    'Resource disk usage percentage',
    ['resource_id', 'resource_name']
)

RESOURCE_NETWORK_IN_BYTES = Gauge(
    'nest_resource_network_in_bytes',
    'Network bytes received',
    ['resource_id', 'resource_name']
)

RESOURCE_NETWORK_OUT_BYTES = Gauge(
    'nest_resource_network_out_bytes',
    'Network bytes transmitted',
    ['resource_id', 'resource_name']
)

RESOURCE_CONNECTIONS = Gauge(
    'nest_resource_connections',
    'Active connections',
    ['resource_id', 'resource_name', 'connection_type']
)

RESOURCE_CACHE_HIT_RATIO = Gauge(
    'nest_resource_cache_hit_ratio',
    'Cache hit ratio percentage',
    ['resource_id', 'resource_name']
)

RESOURCE_RISK_LEVEL = Gauge(
    'nest_resource_risk_level',
    'Resource risk level (0=low, 1=medium, 2=high, 3=critical)',
    ['resource_id', 'resource_name']
)

STATS_COLLECTION_ERRORS = Counter(
    'nest_stats_collection_errors_total',
    'Total statistics collection errors',
    ['resource_id', 'resource_type']
)

STATS_COLLECTION_DURATION = Gauge(
    'nest_stats_collection_duration_seconds',
    'Statistics collection duration',
    ['operation']
)


@dataclass
class RiskFactors:
    """Risk assessment factors for a resource."""
    disk_usage_percent: Optional[float] = None
    memory_percent: Optional[float] = None
    connection_saturation: Optional[float] = None
    cpu_percent: Optional[float] = None
    factors: List[str] = field(default_factory=list)

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary for storage."""
        return {
            'disk_usage_percent': self.disk_usage_percent,
            'memory_percent': self.memory_percent,
            'connection_saturation': self.connection_saturation,
            'cpu_percent': self.cpu_percent,
            'factors': self.factors,
        }


class StatsCollectorException(Exception):
    """Base exception for stats collector errors."""
    pass


class StatsCollector:
    """Collects operational metrics from all managed resources.

    Supports both Kubernetes-based resources (using Metrics API) and external
    resources (using connector-specific stat collection methods).

    Periodically:
    1. Queries all active resources from database
    2. For each resource, collects appropriate metrics based on type
    3. Calculates risk assessment
    4. Stores metrics in database
    5. Updates Prometheus gauges for monitoring
    """

    def __init__(
        self,
        db,
        k8s_client=None,
        interval_seconds: int = 60,
        max_workers: int = 5,
    ):
        """Initialize the statistics collector.

        Args:
            db: PyDAL database instance
            k8s_client: Kubernetes client (optional, lazy-loaded if needed)
            interval_seconds: Collection interval in seconds (default: 60)
            max_workers: Maximum concurrent collection operations (default: 5)
        """
        self.db = db
        self._k8s_client = k8s_client
        self.interval_seconds = interval_seconds
        self.max_workers = max_workers
        self._running = False
        self._thread: Optional[threading.Thread] = None
        self._stop_event = threading.Event()

    @property
    def k8s_client(self):
        """Lazy-load Kubernetes client."""
        if self._k8s_client is None:
            try:
                from apps.manager.lib.k8s_client import get_kubernetes_client
                self._k8s_client = get_kubernetes_client()
            except Exception as e:
                logger.warning(f"Failed to initialize Kubernetes client: {e}")
                return None
        return self._k8s_client

    def start(self) -> None:
        """Start the statistics collector worker thread."""
        if self._running:
            logger.warning("Stats collector is already running")
            return

        self._running = True
        self._stop_event.clear()
        self._thread = threading.Thread(target=self.run, daemon=True)
        self._thread.start()
        logger.info(f"Stats collector started (interval: {self.interval_seconds}s)")

    def stop(self, timeout: int = 10) -> bool:
        """Stop the statistics collector worker.

        Args:
            timeout: Maximum time to wait for worker to stop

        Returns:
            True if stopped successfully, False if timeout occurred
        """
        if not self._running:
            logger.warning("Stats collector is not running")
            return True

        logger.info("Stopping stats collector...")
        self._stop_event.set()
        self._running = False

        if self._thread:
            self._thread.join(timeout=timeout)
            if self._thread.is_alive():
                logger.error(f"Stats collector did not stop within {timeout}s")
                return False

        logger.info("Stats collector stopped")
        return True

    def run(self) -> None:
        """Main worker loop for periodic statistics collection.

        Runs until stop() is called, collecting stats every interval_seconds.
        """
        logger.info(f"Stats collector worker loop started")

        while not self._stop_event.is_set():
            try:
                start_time = time.time()
                self.collect_all_stats()
                elapsed = time.time() - start_time
                STATS_COLLECTION_DURATION.labels(operation='collect_all_stats').set(elapsed)

                # Sleep for remaining interval (or 0 if collection took longer)
                remaining = max(0, self.interval_seconds - elapsed)
                self._stop_event.wait(remaining)

            except Exception as e:
                logger.error(f"Error in stats collection loop: {e}", exc_info=True)
                self._stop_event.wait(10)  # Wait before retrying

    def collect_all_stats(self) -> None:
        """Collect statistics for all active resources.

        Queries resources with lifecycle_mode in ('full', 'partial') and active status,
        then collects appropriate metrics for each resource type.
        """
        try:
            # Query all active resources in full or partial lifecycle mode
            resources = self.db(
                (self.db.resources.status == 'active') &
                (self.db.resources.lifecycle_mode.belongs(['full', 'partial'])) &
                (self.db.resources.deleted_at == None)
            ).select()

            logger.debug(f"Collecting stats for {len(resources)} resources")

            for resource in resources:
                try:
                    self.collect_resource_stats(resource)
                except Exception as e:
                    logger.error(
                        f"Failed to collect stats for resource {resource.id} "
                        f"({resource.name}): {e}",
                        exc_info=True
                    )
                    STATS_COLLECTION_ERRORS.labels(
                        resource_id=str(resource.id),
                        resource_type=resource.resource_type_id
                    ).inc()

        except Exception as e:
            logger.error(f"Failed to query resources: {e}", exc_info=True)

    def collect_resource_stats(self, resource: Any) -> None:
        """Collect statistics for a single resource.

        Determines resource type (Kubernetes or external) and collects
        appropriate metrics using the corresponding method.

        Args:
            resource: Resource record from database
        """
        resource_id = resource.id
        resource_name = resource.name

        logger.debug(f"Collecting stats for resource {resource_name} (ID: {resource_id})")

        try:
            # Determine if resource is Kubernetes-based
            is_k8s = bool(resource.k8s_namespace and resource.k8s_resource_name)

            if is_k8s:
                metrics = self._collect_k8s_metrics(resource)
            else:
                metrics = self._collect_external_metrics(resource)

            if not metrics:
                logger.warning(f"No metrics collected for resource {resource_name}")
                return

            # Calculate risk assessment
            risk_level, risk_factors = self.calculate_risk_level(metrics)

            # Store metrics in database
            self.db.resource_stats.insert(
                resource_id=resource_id,
                timestamp=datetime.now(),
                metrics=metrics,
                risk_level=risk_level,
                risk_factors=risk_factors.to_dict(),
            )
            self.db.commit()

            # Export to Prometheus
            self.export_prometheus_metrics(resource, metrics, risk_level)

            logger.debug(f"Stats collected for resource {resource_name}: "
                        f"risk_level={risk_level}")

        except Exception as e:
            logger.error(f"Error collecting stats for resource {resource_name}: {e}",
                        exc_info=True)
            raise

    def _collect_k8s_metrics(self, resource: Any) -> Optional[Dict[str, Any]]:
        """Collect metrics from Kubernetes Metrics API for a pod/container.

        Args:
            resource: Resource record with k8s details

        Returns:
            Dictionary with collected metrics or None if collection failed
        """
        if not self.k8s_client:
            logger.warning(f"Kubernetes client not available for resource {resource.name}")
            return None

        try:
            namespace = resource.k8s_namespace
            pod_name = resource.k8s_resource_name

            # Get pod metrics using Kubernetes Metrics API
            # Note: Requires metrics-server to be installed in the cluster
            try:
                from kubernetes import client
                custom_api = client.CustomObjectsApi()

                metric_pod = custom_api.get_namespaced_custom_object(
                    group="metrics.k8s.io",
                    version="v1beta1",
                    namespace=namespace,
                    plural="pods",
                    name=pod_name,
                )

                metrics = self._parse_k8s_metrics(metric_pod)
                return metrics

            except Exception as e:
                logger.warning(
                    f"Failed to get Kubernetes metrics for {namespace}/{pod_name}: {e}"
                )
                return None

        except Exception as e:
            logger.error(f"Error collecting K8s metrics for resource {resource.name}: {e}",
                        exc_info=True)
            return None

    def _parse_k8s_metrics(self, metric_pod: Dict[str, Any]) -> Dict[str, Any]:
        """Parse Kubernetes metrics API response.

        Args:
            metric_pod: Raw metrics API response

        Returns:
            Normalized metrics dictionary
        """
        metrics = {
            'timestamp': datetime.now().isoformat(),
            'cpu_percent': 0.0,
            'memory_bytes': 0,
            'memory_percent': 0.0,
        }

        try:
            containers = metric_pod.get('containers', [])
            if not containers:
                return metrics

            # Sum metrics across all containers in pod
            total_cpu_m = 0  # millicores
            total_memory_bytes = 0

            for container in containers:
                usage = container.get('usage', {})

                # CPU: convert millicores to percentage (assuming 1000m = 100%)
                cpu_str = usage.get('cpu', '0m')
                if cpu_str.endswith('m'):
                    total_cpu_m += int(cpu_str[:-1])
                elif cpu_str.endswith('n'):
                    total_cpu_m += int(cpu_str[:-1]) / 1_000_000

                # Memory: convert to bytes
                memory_str = usage.get('memory', '0Ki')
                total_memory_bytes += self._parse_k8s_quantity(memory_str)

            # Convert millicores to percentage (assuming 1000m per core, using single core as base)
            metrics['cpu_percent'] = min(100.0, (total_cpu_m / 1000.0) * 100)
            metrics['memory_bytes'] = total_memory_bytes

        except Exception as e:
            logger.warning(f"Error parsing K8s metrics: {e}")

        return metrics

    def _parse_k8s_quantity(self, quantity: str) -> int:
        """Parse Kubernetes resource quantity string to bytes.

        Args:
            quantity: Kubernetes quantity string (e.g., "128Mi", "512Gi")

        Returns:
            Value in bytes
        """
        if not quantity:
            return 0

        multipliers = {
            'Ki': 1024,
            'Mi': 1024 ** 2,
            'Gi': 1024 ** 3,
            'Ti': 1024 ** 4,
            'k': 1000,
            'M': 1000 ** 2,
            'G': 1000 ** 3,
            'T': 1000 ** 4,
        }

        for suffix, multiplier in multipliers.items():
            if quantity.endswith(suffix):
                try:
                    value = float(quantity[:-len(suffix)])
                    return int(value * multiplier)
                except ValueError:
                    return 0

        # Try plain number
        try:
            return int(quantity)
        except ValueError:
            return 0

    def _collect_external_metrics(self, resource: Any) -> Optional[Dict[str, Any]]:
        """Collect metrics from external resource using appropriate connector.

        Args:
            resource: Resource record with connection info

        Returns:
            Dictionary with collected metrics or None if collection failed
        """
        try:
            resource_type_id = resource.resource_type_id
            resource_type = self.db.resource_types[resource_type_id]

            if not resource_type:
                logger.warning(f"Resource type not found: {resource_type_id}")
                return None

            # Import and instantiate appropriate connector
            connector = self._get_resource_connector(resource, resource_type)
            if not connector:
                return None

            # Collect stats using connector's method
            if hasattr(connector, 'collect_stats'):
                connector_stats = connector.collect_stats()
                return self._normalize_external_metrics(connector_stats, resource_type.name)
            else:
                logger.warning(
                    f"Connector for {resource_type.name} does not support collect_stats"
                )
                return None

        except Exception as e:
            logger.error(
                f"Error collecting external metrics for resource {resource.name}: {e}",
                exc_info=True
            )
            return None

    def _get_resource_connector(self, resource: Any, resource_type: Any):
        """Instantiate appropriate connector for resource type.

        Args:
            resource: Resource record
            resource_type: Resource type record

        Returns:
            Instantiated connector or None if connector not available
        """
        try:
            resource_type_name = resource_type.name.lower()

            if resource_type_name == 'postgresql':
                from apps.manager.lib.resource_connectors.postgresql import PostgreSQLConnector
                return PostgreSQLConnector(resource.connection_info, resource.credentials)

            elif resource_type_name == 'mariadb':
                from apps.manager.lib.resource_connectors.mariadb import MariaDBConnector
                return MariaDBConnector(resource.connection_info, resource.credentials)

            elif resource_type_name == 'redis':
                from apps.manager.lib.resource_connectors.redis import RedisConnector
                return RedisConnector(resource.connection_info, resource.credentials)

            elif resource_type_name == 'ceph':
                from apps.manager.lib.resource_connectors.ceph import CephConnector
                return CephConnector(resource.connection_info, resource.credentials)

            elif resource_type_name == 'san':
                from apps.manager.lib.resource_connectors.san import SANConnector
                return SANConnector(resource.connection_info, resource.credentials)

            else:
                logger.warning(f"No connector available for resource type: {resource_type_name}")
                return None

        except ImportError as e:
            logger.warning(f"Failed to import connector: {e}")
            return None
        except Exception as e:
            logger.error(f"Failed to instantiate connector: {e}")
            return None

    def _normalize_external_metrics(
        self,
        connector_stats: Dict[str, Any],
        resource_type: str
    ) -> Dict[str, Any]:
        """Normalize connector-specific metrics to standard format.

        Args:
            connector_stats: Raw metrics from connector
            resource_type: Type of resource (postgresql, redis, etc)

        Returns:
            Normalized metrics dictionary
        """
        metrics = {
            'timestamp': datetime.now().isoformat(),
            'resource_type': resource_type,
        }

        # Database-specific metrics (PostgreSQL, MariaDB)
        if resource_type.lower() in ['postgresql', 'mariadb', 'mysql']:
            metrics['connections'] = connector_stats.get('connections', {})
            metrics['database_size_bytes'] = connector_stats.get('database_size_bytes', 0)
            metrics['cache_hit_ratio'] = connector_stats.get('cache_hit_ratio', 0.0)
            metrics['transaction_stats'] = connector_stats.get('transaction_stats', {})
            metrics['query_performance'] = connector_stats.get('query_performance', {})

        # Redis/Cache-specific metrics
        elif resource_type.lower() in ['redis', 'valkey']:
            metrics['used_memory_bytes'] = connector_stats.get('used_memory_bytes', 0)
            metrics['used_memory_percent'] = connector_stats.get('used_memory_percent', 0.0)
            metrics['connected_clients'] = connector_stats.get('connected_clients', 0)
            metrics['cache_hit_ratio'] = connector_stats.get('keyspace_hits', 0) / max(
                1,
                connector_stats.get('keyspace_hits', 0) + connector_stats.get('keyspace_misses', 0)
            ) * 100

        # Storage-specific metrics (Ceph, SAN)
        elif resource_type.lower() in ['ceph', 'san']:
            metrics['used_bytes'] = connector_stats.get('used_bytes', 0)
            metrics['available_bytes'] = connector_stats.get('available_bytes', 0)
            metrics['total_bytes'] = connector_stats.get('total_bytes', 0)
            if metrics['total_bytes'] > 0:
                metrics['disk_usage_percent'] = (
                    metrics['used_bytes'] / metrics['total_bytes'] * 100
                )

        # Include raw stats for inspection
        metrics['raw_stats'] = connector_stats

        return metrics

    def calculate_risk_level(self, metrics: Dict[str, Any]) -> tuple[str, RiskFactors]:
        """Calculate operational risk level based on metrics.

        Risk thresholds:
        - Disk usage > 95%: CRITICAL
        - Disk usage > 85%: HIGH
        - Memory > 90%: HIGH
        - Connection saturation > 80%: MEDIUM
        - CPU > 85%: MEDIUM (if sustained)

        Args:
            metrics: Normalized metrics dictionary

        Returns:
            Tuple of (risk_level_string, RiskFactors object)
        """
        risk_factors = RiskFactors()
        risk_level = 'low'

        # Check disk usage
        disk_usage = metrics.get('disk_usage_percent')
        if disk_usage is not None:
            risk_factors.disk_usage_percent = disk_usage
            if disk_usage > 95:
                risk_level = 'critical'
                risk_factors.factors.append('Disk usage critical (>95%)')
            elif disk_usage > 85:
                risk_level = 'high' if risk_level == 'low' else risk_level
                risk_factors.factors.append('Disk usage high (>85%)')

        # Check memory usage
        memory_percent = metrics.get('memory_percent')
        if memory_percent is not None:
            risk_factors.memory_percent = memory_percent
            if memory_percent > 90:
                if risk_level != 'critical':
                    risk_level = 'high'
                risk_factors.factors.append('Memory usage high (>90%)')
            elif memory_percent > 85:
                if risk_level not in ['critical', 'high']:
                    risk_level = 'medium'
                risk_factors.factors.append('Memory usage moderate (>85%)')

        # Check connection saturation (for databases)
        # Support both dict format (connections: {total, active}) and direct metric
        connection_saturation = None
        connections = metrics.get('connections', {})
        if isinstance(connections, dict) and connections:
            total_conns = connections.get('total', 0)
            active_conns = connections.get('active', 0)
            if total_conns > 0:
                connection_saturation = (active_conns / total_conns) * 100

        # If we didn't get it from the dict, check for direct connection_saturation metric
        if connection_saturation is None:
            connection_saturation = metrics.get('connection_saturation')

        if connection_saturation is not None:
            risk_factors.connection_saturation = connection_saturation
            if connection_saturation > 80:
                if risk_level not in ['critical', 'high']:
                    risk_level = 'medium'
                risk_factors.factors.append(f'Connection saturation high ({connection_saturation:.1f}%)')

        # Check CPU usage
        cpu_percent = metrics.get('cpu_percent')
        if cpu_percent is not None:
            risk_factors.cpu_percent = cpu_percent
            if cpu_percent > 85:
                if risk_level not in ['critical', 'high']:
                    risk_level = 'medium'
                risk_factors.factors.append(f'CPU usage high ({cpu_percent:.1f}%)')

        return risk_level, risk_factors

    def export_prometheus_metrics(
        self,
        resource: Any,
        metrics: Dict[str, Any],
        risk_level: str
    ) -> None:
        """Export collected metrics to Prometheus.

        Updates Prometheus gauges with current metric values for monitoring
        and alerting.

        Args:
            resource: Resource record
            metrics: Collected metrics dictionary
            risk_level: Calculated risk level (low, medium, high, critical)
        """
        resource_id = str(resource.id)
        resource_name = resource.name

        try:
            # CPU usage
            if 'cpu_percent' in metrics:
                RESOURCE_CPU_PERCENT.labels(
                    resource_id=resource_id,
                    resource_name=resource_name
                ).set(metrics['cpu_percent'])

            # Memory usage
            if 'memory_bytes' in metrics:
                RESOURCE_MEMORY_BYTES.labels(
                    resource_id=resource_id,
                    resource_name=resource_name
                ).set(metrics['memory_bytes'])

            if 'memory_percent' in metrics:
                RESOURCE_MEMORY_PERCENT.labels(
                    resource_id=resource_id,
                    resource_name=resource_name
                ).set(metrics['memory_percent'])

            # Disk usage
            if 'disk_usage_percent' in metrics:
                RESOURCE_DISK_USAGE_PERCENT.labels(
                    resource_id=resource_id,
                    resource_name=resource_name
                ).set(metrics['disk_usage_percent'])

            # Network I/O
            if 'network_in_bytes' in metrics:
                RESOURCE_NETWORK_IN_BYTES.labels(
                    resource_id=resource_id,
                    resource_name=resource_name
                ).set(metrics['network_in_bytes'])

            if 'network_out_bytes' in metrics:
                RESOURCE_NETWORK_OUT_BYTES.labels(
                    resource_id=resource_id,
                    resource_name=resource_name
                ).set(metrics['network_out_bytes'])

            # Connections
            connections = metrics.get('connections', {})
            if isinstance(connections, dict):
                for conn_type, count in connections.items():
                    RESOURCE_CONNECTIONS.labels(
                        resource_id=resource_id,
                        resource_name=resource_name,
                        connection_type=conn_type
                    ).set(count)

            # Cache hit ratio
            if 'cache_hit_ratio' in metrics:
                RESOURCE_CACHE_HIT_RATIO.labels(
                    resource_id=resource_id,
                    resource_name=resource_name
                ).set(metrics['cache_hit_ratio'])

            # Risk level (convert to numeric: low=0, medium=1, high=2, critical=3)
            risk_level_map = {'low': 0, 'medium': 1, 'high': 2, 'critical': 3}
            risk_numeric = risk_level_map.get(risk_level, 0)
            RESOURCE_RISK_LEVEL.labels(
                resource_id=resource_id,
                resource_name=resource_name
            ).set(risk_numeric)

        except Exception as e:
            logger.error(f"Error exporting Prometheus metrics for {resource_name}: {e}",
                        exc_info=True)
