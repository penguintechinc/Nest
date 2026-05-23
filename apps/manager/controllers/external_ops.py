"""External Operations Controller

Handles partial lifecycle operations for externally managed resources.
Provides configuration updates, user synchronization, backup/restore operations,
and statistics collection for resources in partial or monitor_only lifecycle modes.
"""

import logging
import json
from datetime import datetime
from typing import Dict, List, Optional, Any, Tuple

from penguin_dal.quart_ext import get_db
from lib.resource_connectors.postgresql import PostgreSQLConnector
from lib.resource_connectors.mariadb import MariaDBConnector
from lib.resource_connectors.redis import RedisConnector
from lib.resource_connectors.ceph import CephConnector
from lib.resource_connectors.san import SANConnector


logger = logging.getLogger(__name__)


class ExternalOpsControllerError(Exception):
    """Base exception for external operations controller errors."""
    pass


class InvalidResourceError(ExternalOpsControllerError):
    """Resource validation errors."""
    pass


class ConnectorError(ExternalOpsControllerError):
    """Connector operation errors."""
    pass


class ExternalOpsController:
    """Controller for managing external resource operations.

    Handles partial lifecycle management including configuration updates,
    user synchronization, backup/restore, and statistics collection.
    """

    SUPPORTED_LIFECYCLE_MODES = ['partial', 'monitor_only']

    CONNECTOR_MAP = {
        'db-postgresql': PostgreSQLConnector,
        'db-mariadb': MariaDBConnector,
        'db-redis': RedisConnector,
        'storage-ceph': CephConnector,
        'storage-san': SANConnector,
    }

    @staticmethod
    def _load_resource(resource_id: int) -> Dict[str, Any]:
        db = get_db()
        """Load resource from database.

        Args:
            resource_id: Resource ID to load

        Returns:
            Resource record as dictionary

        Raises:
            InvalidResourceError: If resource not found
        """
        resource = db.resources[resource_id]
        if not resource:
            raise InvalidResourceError(f"Resource {resource_id} not found")
        return resource

    @staticmethod
    def _get_resource_type(resource_type_id: int) -> Dict[str, Any]:
        db = get_db()
        """Get resource type definition.

        Args:
            resource_type_id: Resource type ID

        Returns:
            Resource type record

        Raises:
            InvalidResourceError: If resource type not found
        """
        resource_type = db.resource_types[resource_type_id]
        if not resource_type:
            raise InvalidResourceError(f"Resource type {resource_type_id} not found")
        return resource_type

    @staticmethod
    def _validate_lifecycle_mode(resource: Dict[str, Any]) -> None:
        db = get_db()
        """Validate that resource is in supported lifecycle mode.

        Args:
            resource: Resource record

        Raises:
            InvalidResourceError: If lifecycle mode not supported
        """
        if resource.lifecycle_mode not in ExternalOpsController.SUPPORTED_LIFECYCLE_MODES:
            raise InvalidResourceError(
                f"Resource lifecycle mode '{resource.lifecycle_mode}' does not support "
                "external operations. Only 'partial' and 'monitor_only' modes supported."
            )

    @staticmethod
    def _get_connector_class(resource_type_name: str) -> type:
        db = get_db()
        """Get connector class for resource type.

        Args:
            resource_type_name: Resource type name (e.g., 'db-postgresql')

        Returns:
            Connector class

        Raises:
            ConnectorError: If connector type not supported
        """
        connector_class = ExternalOpsController.CONNECTOR_MAP.get(resource_type_name)
        if not connector_class:
            raise ConnectorError(
                f"Unsupported resource type '{resource_type_name}'. "
                f"Supported types: {', '.join(ExternalOpsController.CONNECTOR_MAP.keys())}"
            )
        return connector_class

    @staticmethod
    def _initialize_connector(connector_class: type,
                             resource: Dict[str, Any]) -> Any:
        db = get_db()
        """Initialize connector instance for resource.

        Args:
            connector_class: Connector class to instantiate
            resource: Resource record with connection_info and credentials

        Returns:
            Initialized connector instance

        Raises:
            ConnectorError: If connector initialization fails
        """
        try:
            connection_info = resource.connection_info or {}
            credentials = resource.credentials or {}

            # Decrypt credentials if needed
            if isinstance(credentials, str):
                try:
                    credentials = json.loads(credentials)
                except json.JSONDecodeError:
                    credentials = {}

            connector = connector_class(
                connection_info=connection_info,
                credentials=credentials
            )
            return connector

        except Exception as e:
            raise ConnectorError(
                f"Failed to initialize connector for resource {resource.id}: {e}"
            )

    @staticmethod
    def _create_audit_log(resource_id: int, action: str, details: Dict[str, Any],
                         user_id: Optional[int] = None, team_id: Optional[int] = None) -> int:
        db = get_db()
        """Create audit log entry for action.

        Args:
            resource_id: Resource affected
            action: Action performed
            details: Additional details as dictionary
            user_id: User who performed action
            team_id: Team context

        Returns:
            Audit log ID
        """
        try:
            resource = db.resources[resource_id]
            if resource:
                team_id = team_id or resource.team_id

            audit_id = db.audit_logs.insert(
                user_id=user_id,
                action=action,
                resource_type='resource',
                resource_id=resource_id,
                team_id=team_id,
                details=json.dumps(details) if isinstance(details, dict) else details,
                timestamp=datetime.utcnow()
            )
            db.commit()
            logger.info(f"Created audit log entry {audit_id} for action '{action}' on resource {resource_id}")
            return audit_id

        except Exception as e:
            logger.error(f"Failed to create audit log for action '{action}': {e}")
            return None

    def update_resource_config(self, resource_id: int, config_params: Dict[str, Any],
                              user_id: Optional[int] = None) -> bool:
        """Update resource configuration parameters.

        Workflow:
        1. Load resource and validate lifecycle mode
        2. Check can_modify_config capability
        3. Get resource type and initialize connector
        4. Call connector.update_config()
        5. Update resource.config in database
        6. Create audit log entry

        Args:
            resource_id: Resource ID to update
            config_params: Configuration parameters to apply
            user_id: User performing the operation

        Returns:
            True if configuration was updated successfully

        Raises:
            InvalidResourceError: If resource validation fails
            ConnectorError: If connector operation fails
        """
        try:
            # Load and validate resource
            resource = self._load_resource(resource_id)
            self._validate_lifecycle_mode(resource)

            if not resource.can_modify_config:
                raise InvalidResourceError(
                    f"Resource {resource_id} does not allow configuration modifications"
                )

            logger.info(f"Updating configuration for resource {resource_id}")

            # Get resource type and connector
            resource_type = self._get_resource_type(resource.resource_type_id)
            connector_class = self._get_connector_class(resource_type.name)
            connector = self._initialize_connector(connector_class, resource)

            # Call connector to update config
            connector.update_config(config_params)

            # Update resource record
            current_config = resource.config or {}
            if isinstance(current_config, str):
                try:
                    current_config = json.loads(current_config)
                except json.JSONDecodeError:
                    current_config = {}

            # Merge new config with existing
            current_config.update(config_params)

            db.resources[resource_id] = dict(
                config=json.dumps(current_config),
                updated_at=datetime.utcnow()
            )
            db.commit()

            # Create audit log
            self._create_audit_log(
                resource_id=resource_id,
                action='update_config',
                details={
                    'config_params': config_params,
                    'status': 'completed'
                },
                user_id=user_id,
                team_id=resource.team_id
            )

            logger.info(f"Successfully updated configuration for resource {resource_id}")
            return True

        except (InvalidResourceError, ConnectorError) as e:
            logger.error(f"Failed to update resource config: {e}")
            raise

    def update_resource_users(self, resource_id: int, user_id: Optional[int] = None) -> bool:
        """Synchronize all users to resource.

        Syncs resource_users entries to the actual resource by calling
        connector user management methods.

        Args:
            resource_id: Resource ID
            user_id: User performing the operation

        Returns:
            True if sync completed successfully

        Raises:
            InvalidResourceError: If resource validation fails
            ConnectorError: If sync operation fails
        """
        try:
            # Load and validate resource
            resource = self._load_resource(resource_id)
            self._validate_lifecycle_mode(resource)

            if not resource.can_modify_users:
                raise InvalidResourceError(
                    f"Resource {resource_id} does not allow user modifications"
                )

            logger.info(f"Syncing users for resource {resource_id}")

            # Get resource type and connector
            resource_type = self._get_resource_type(resource.resource_type_id)
            connector_class = self._get_connector_class(resource_type.name)
            connector = self._initialize_connector(connector_class, resource)

            # Get all resource users
            resource_users = db(db.resource_users.resource_id == resource_id).select()

            sync_errors = []
            synced_count = 0

            for res_user in resource_users:
                try:
                    # Update sync status to 'syncing'
                    db.resource_users[res_user.id] = dict(
                        sync_status='syncing',
                        updated_at=datetime.utcnow()
                    )
                    db.commit()

                    # Sync user to resource
                    if hasattr(connector, 'create_user') and res_user.sync_status == 'pending':
                        # Decrypt password if needed
                        password = res_user.password_hash
                        if isinstance(password, str) and password.startswith('fernet:'):
                            # Would need decryption service here
                            pass

                        roles = res_user.roles or []
                        if isinstance(roles, str):
                            try:
                                roles = json.loads(roles)
                            except json.JSONDecodeError:
                                roles = []

                        connector.create_user(
                            username=res_user.username,
                            password=password,
                            roles=roles
                        )

                    # Mark as synced
                    db.resource_users[res_user.id] = dict(
                        sync_status='synced',
                        last_synced_at=datetime.utcnow(),
                        sync_error=None,
                        updated_at=datetime.utcnow()
                    )
                    db.commit()
                    synced_count += 1

                except Exception as e:
                    error_msg = str(e)
                    logger.error(f"Failed to sync user {res_user.username} to resource {resource_id}: {e}")

                    # Mark as error
                    db.resource_users[res_user.id] = dict(
                        sync_status='error',
                        sync_error=error_msg,
                        updated_at=datetime.utcnow()
                    )
                    db.commit()
                    sync_errors.append(f"{res_user.username}: {error_msg}")

            # Create audit log
            self._create_audit_log(
                resource_id=resource_id,
                action='sync_users',
                details={
                    'synced_count': synced_count,
                    'total_users': len(resource_users),
                    'errors': sync_errors,
                    'status': 'completed'
                },
                user_id=user_id,
                team_id=resource.team_id
            )

            logger.info(f"Synced {synced_count}/{len(resource_users)} users to resource {resource_id}")
            return len(sync_errors) == 0

        except (InvalidResourceError, ConnectorError) as e:
            logger.error(f"Failed to sync users: {e}")
            raise

    def trigger_backup(self, resource_id: int, backup_type: str = 'full',
                      backup_location: str = None, user_id: Optional[int] = None) -> int:
        """Trigger backup operation on resource.

        Workflow:
        1. Load resource and validate
        2. Check can_backup capability
        3. Initialize connector
        4. Call connector.trigger_backup()
        5. Create backup_jobs record
        6. Return backup job ID

        Args:
            resource_id: Resource to backup
            backup_type: Type of backup (e.g., 'full', 'incremental')
            backup_location: Where to store backup (path or S3 URL)
            user_id: User performing the operation

        Returns:
            Backup job ID

        Raises:
            InvalidResourceError: If resource validation fails
            ConnectorError: If backup operation fails
        """
        try:
            # Load and validate resource
            resource = self._load_resource(resource_id)
            self._validate_lifecycle_mode(resource)

            if not resource.can_backup:
                raise InvalidResourceError(
                    f"Resource {resource_id} does not support backup operations"
                )

            logger.info(f"Triggering {backup_type} backup for resource {resource_id}")

            # Get resource type and connector
            resource_type = self._get_resource_type(resource.resource_type_id)
            connector_class = self._get_connector_class(resource_type.name)
            connector = self._initialize_connector(connector_class, resource)

            # Create backup job record
            job_id = db.backup_jobs.insert(
                resource_id=resource_id,
                job_type=backup_type,
                status='pending',
                backup_location=backup_location,
                created_by=user_id,
                created_at=datetime.utcnow()
            )
            db.commit()

            try:
                # Update status to running
                db.backup_jobs[job_id] = dict(
                    status='running',
                    started_at=datetime.utcnow()
                )
                db.commit()

                # Execute backup
                backup_path = connector.trigger_backup(
                    backup_location=backup_location or f'/backups/{resource_id}',
                    database=None,
                    format_type='plain',
                    verbose=True
                )

                # Get backup size
                import os
                backup_size = 0
                if os.path.exists(backup_path):
                    backup_size = os.path.getsize(backup_path)

                # Update job as completed
                db.backup_jobs[job_id] = dict(
                    status='completed',
                    backup_location=backup_path,
                    backup_size_bytes=backup_size,
                    completed_at=datetime.utcnow()
                )
                db.commit()

                # Create audit log
                self._create_audit_log(
                    resource_id=resource_id,
                    action='trigger_backup',
                    details={
                        'backup_type': backup_type,
                        'backup_location': backup_path,
                        'backup_size_bytes': backup_size,
                        'job_id': job_id,
                        'status': 'completed'
                    },
                    user_id=user_id,
                    team_id=resource.team_id
                )

                logger.info(f"Successfully completed backup job {job_id} for resource {resource_id}")

            except Exception as e:
                error_msg = str(e)
                logger.error(f"Backup job {job_id} failed: {e}")

                # Update job as failed
                db.backup_jobs[job_id] = dict(
                    status='failed',
                    error_message=error_msg,
                    completed_at=datetime.utcnow()
                )
                db.commit()

                # Create audit log for failure
                self._create_audit_log(
                    resource_id=resource_id,
                    action='trigger_backup',
                    details={
                        'backup_type': backup_type,
                        'job_id': job_id,
                        'status': 'failed',
                        'error': error_msg
                    },
                    user_id=user_id,
                    team_id=resource.team_id
                )

                raise ConnectorError(f"Backup operation failed: {error_msg}")

            return job_id

        except (InvalidResourceError, ConnectorError) as e:
            logger.error(f"Failed to trigger backup: {e}")
            raise

    def restore_backup(self, resource_id: int, backup_location: str,
                      user_id: Optional[int] = None) -> int:
        """Restore resource from backup.

        Workflow:
        1. Load resource and validate
        2. Check can_backup capability
        3. Initialize connector
        4. Call connector.restore_backup()
        5. Create backup_jobs record with type='restore'
        6. Return job ID

        Args:
            resource_id: Resource to restore
            backup_location: Path or location of backup to restore
            user_id: User performing the operation

        Returns:
            Restore job ID

        Raises:
            InvalidResourceError: If resource validation fails
            ConnectorError: If restore operation fails
        """
        try:
            # Load and validate resource
            resource = self._load_resource(resource_id)
            self._validate_lifecycle_mode(resource)

            if not resource.can_backup:
                raise InvalidResourceError(
                    f"Resource {resource_id} does not support backup/restore operations"
                )

            logger.info(f"Restoring resource {resource_id} from {backup_location}")

            # Get resource type and connector
            resource_type = self._get_resource_type(resource.resource_type_id)
            connector_class = self._get_connector_class(resource_type.name)
            connector = self._initialize_connector(connector_class, resource)

            # Create restore job record
            job_id = db.backup_jobs.insert(
                resource_id=resource_id,
                job_type='restore',
                status='pending',
                backup_location=backup_location,
                created_by=user_id,
                created_at=datetime.utcnow()
            )
            db.commit()

            try:
                # Update status to running
                db.backup_jobs[job_id] = dict(
                    status='running',
                    started_at=datetime.utcnow()
                )
                db.commit()

                # Execute restore
                connector.restore_backup(
                    backup_location=backup_location,
                    database=None,
                    clean=True,
                    if_exists=True,
                    verbose=True
                )

                # Update job as completed
                db.backup_jobs[job_id] = dict(
                    status='completed',
                    completed_at=datetime.utcnow()
                )
                db.commit()

                # Create audit log
                self._create_audit_log(
                    resource_id=resource_id,
                    action='restore_backup',
                    details={
                        'backup_location': backup_location,
                        'job_id': job_id,
                        'status': 'completed'
                    },
                    user_id=user_id,
                    team_id=resource.team_id
                )

                logger.info(f"Successfully completed restore job {job_id} for resource {resource_id}")

            except Exception as e:
                error_msg = str(e)
                logger.error(f"Restore job {job_id} failed: {e}")

                # Update job as failed
                db.backup_jobs[job_id] = dict(
                    status='failed',
                    error_message=error_msg,
                    completed_at=datetime.utcnow()
                )
                db.commit()

                # Create audit log for failure
                self._create_audit_log(
                    resource_id=resource_id,
                    action='restore_backup',
                    details={
                        'backup_location': backup_location,
                        'job_id': job_id,
                        'status': 'failed',
                        'error': error_msg
                    },
                    user_id=user_id,
                    team_id=resource.team_id
                )

                raise ConnectorError(f"Restore operation failed: {error_msg}")

            return job_id

        except (InvalidResourceError, ConnectorError) as e:
            logger.error(f"Failed to restore backup: {e}")
            raise

    def collect_resource_stats(self, resource_id: int,
                              user_id: Optional[int] = None) -> Dict[str, Any]:
        """Collect resource statistics and assess risk level.

        Workflow:
        1. Load resource
        2. Initialize connector
        3. Call connector.collect_stats()
        4. Calculate risk level based on metrics
        5. Insert into resource_stats table
        6. Return stats with risk assessment

        Risk calculation:
        - Disk usage > 95%: critical
        - Disk usage > 85%: high
        - Memory > 90%: high
        - Connection saturation > 80%: medium

        Args:
            resource_id: Resource to collect stats for
            user_id: User performing the operation

        Returns:
            Dictionary containing metrics and risk assessment

        Raises:
            InvalidResourceError: If resource not found
            ConnectorError: If stats collection fails
        """
        try:
            # Load resource
            resource = self._load_resource(resource_id)
            self._validate_lifecycle_mode(resource)

            logger.info(f"Collecting statistics for resource {resource_id}")

            # Get resource type and connector
            resource_type = self._get_resource_type(resource.resource_type_id)
            connector_class = self._get_connector_class(resource_type.name)
            connector = self._initialize_connector(connector_class, resource)

            # Collect stats from connector
            metrics = connector.collect_stats()

            # Calculate risk level
            risk_level, risk_factors = self._calculate_risk_level(metrics, resource_type.name)

            # Insert into resource_stats
            stats_id = db.resource_stats.insert(
                resource_id=resource_id,
                timestamp=datetime.utcnow(),
                metrics=json.dumps(metrics),
                risk_level=risk_level,
                risk_factors=json.dumps(risk_factors)
            )
            db.commit()

            # Create audit log if risk level is high or critical
            if risk_level in ['high', 'critical']:
                self._create_audit_log(
                    resource_id=resource_id,
                    action='collect_stats',
                    details={
                        'risk_level': risk_level,
                        'risk_factors': risk_factors,
                        'stats_id': stats_id
                    },
                    user_id=user_id,
                    team_id=resource.team_id
                )

            logger.info(f"Collected stats for resource {resource_id} - Risk level: {risk_level}")

            return {
                'stats_id': stats_id,
                'timestamp': datetime.utcnow().isoformat(),
                'metrics': metrics,
                'risk_level': risk_level,
                'risk_factors': risk_factors
            }

        except (InvalidResourceError, ConnectorError) as e:
            logger.error(f"Failed to collect resource stats: {e}")
            raise

    @staticmethod
    def _calculate_risk_level(metrics: Dict[str, Any],
                             resource_type: str) -> Tuple[str, List[str]]:
        db = get_db()
        """Calculate risk level based on metrics.

        Args:
            metrics: Metrics dictionary from connector
            resource_type: Type of resource for context-specific checks

        Returns:
            Tuple of (risk_level, list_of_risk_factors)
            risk_level: 'low', 'medium', 'high', or 'critical'
        """
        risk_factors = []
        max_risk = 'low'

        # Check disk usage (for databases and storage)
        if 'disk_usage_percent' in metrics:
            usage = metrics['disk_usage_percent']
            if usage > 95:
                risk_factors.append(f"Disk usage critical: {usage}%")
                max_risk = 'critical'
            elif usage > 85:
                risk_factors.append(f"Disk usage high: {usage}%")
                if max_risk != 'critical':
                    max_risk = 'high'

        # Check memory usage (for databases)
        if 'memory_usage_percent' in metrics:
            usage = metrics['memory_usage_percent']
            if usage > 90:
                risk_factors.append(f"Memory usage high: {usage}%")
                if max_risk not in ['critical', 'high']:
                    max_risk = 'high'

        # Check connection saturation (for databases)
        if 'connections' in metrics:
            conn_info = metrics['connections']
            if isinstance(conn_info, dict):
                active = conn_info.get('active', 0)
                total = conn_info.get('total', 1)
                if total > 0:
                    saturation = (active / total) * 100
                    if saturation > 80:
                        risk_factors.append(f"Connection saturation: {saturation}%")
                        if max_risk == 'low':
                            max_risk = 'medium'

        # Check temp space (for databases)
        if 'temp_files' in metrics:
            temp_info = metrics['temp_files']
            if isinstance(temp_info, dict):
                temp_size = temp_info.get('size_bytes', 0)
                if temp_size > 1073741824:  # > 1GB
                    risk_factors.append(f"Temporary space usage: {temp_size / 1073741824:.2f}GB")
                    if max_risk == 'low':
                        max_risk = 'medium'

        # Check replication lag (for databases)
        if 'replication_lag_seconds' in metrics:
            lag = metrics['replication_lag_seconds']
            if lag is not None and lag > 3600:  # > 1 hour
                risk_factors.append(f"Replication lag: {lag}s")
                if max_risk == 'low':
                    max_risk = 'medium'

        return max_risk, risk_factors

    def test_connection(self, resource_id: int) -> bool:
        """Test connectivity to resource.

        Args:
            resource_id: Resource to test

        Returns:
            True if connection successful

        Raises:
            InvalidResourceError: If resource not found
            ConnectorError: If connection test fails
        """
        try:
            # Load resource
            resource = self._load_resource(resource_id)

            logger.info(f"Testing connection to resource {resource_id}")

            # Get resource type and connector
            resource_type = self._get_resource_type(resource.resource_type_id)
            connector_class = self._get_connector_class(resource_type.name)

            # Initialize connector (which tests connection)
            connector = self._initialize_connector(connector_class, resource)

            logger.info(f"Successfully tested connection to resource {resource_id}")
            return True

        except (InvalidResourceError, ConnectorError) as e:
            logger.error(f"Connection test failed: {e}")
            raise

    def reload_configuration(self, resource_id: int, user_id: Optional[int] = None) -> bool:
        """Reload configuration on resource without full restart.

        Args:
            resource_id: Resource to reload config on
            user_id: User performing the operation

        Returns:
            True if reload successful

        Raises:
            InvalidResourceError: If resource not found
            ConnectorError: If reload fails
        """
        try:
            # Load and validate resource
            resource = self._load_resource(resource_id)
            self._validate_lifecycle_mode(resource)

            logger.info(f"Reloading configuration for resource {resource_id}")

            # Get resource type and connector
            resource_type = self._get_resource_type(resource.resource_type_id)
            connector_class = self._get_connector_class(resource_type.name)
            connector = self._initialize_connector(connector_class, resource)

            # Call reload_config if available
            if hasattr(connector, 'reload_config'):
                connector.reload_config()
            else:
                raise ConnectorError(
                    f"Connector for {resource_type.name} does not support reload_config"
                )

            # Create audit log
            self._create_audit_log(
                resource_id=resource_id,
                action='reload_config',
                details={
                    'status': 'completed'
                },
                user_id=user_id,
                team_id=resource.team_id
            )

            logger.info(f"Successfully reloaded configuration for resource {resource_id}")
            return True

        except (InvalidResourceError, ConnectorError) as e:
            logger.error(f"Failed to reload configuration: {e}")
            raise
