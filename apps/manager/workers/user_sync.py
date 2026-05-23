"""
User Synchronization Worker

Synchronizes resource users from NEST database to actual resources.
Handles user creation, updates, and deletion across multiple resource types
including PostgreSQL, MariaDB, Redis, Valkey, Ceph, and SAN resources.

Worker Features:
- Monitors resource_users table for pending and updated users
- Dispatches to appropriate resource connectors
- Handles connection management and error recovery
- Supports graceful shutdown with signal handling
- Implements retry logic and detailed error logging
"""

import os
import sys
import time
import signal
import logging
import json
from datetime import datetime
from typing import Optional, Dict, Any

# Import PyDAL database
from models import db

# Import resource connectors (graceful fallback if not available)
try:
    from lib.resource_connectors.postgresql import PostgreSQLConnector
except ImportError:
    PostgreSQLConnector = None

try:
    from lib.resource_connectors.mariadb import MariaDBConnector
except ImportError:
    MariaDBConnector = None

try:
    from lib.resource_connectors.redis import RedisConnector
except ImportError:
    RedisConnector = None

try:
    from lib.resource_connectors.ceph import CephConnector
except ImportError:
    CephConnector = None

try:
    from lib.resource_connectors.san import SANConnector
except ImportError:
    SANConnector = None


# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class UserSyncWorker:
    """
    Worker for synchronizing resource users from NEST database to actual resources.

    Implements a main loop that queries for pending users and syncs them to their
    respective resources using resource-specific connectors.
    """

    def __init__(self, sleep_interval: int = 30, batch_size: int = 10):
        """
        Initialize the UserSyncWorker.

        Args:
            sleep_interval: Seconds to sleep between sync cycles (default: 30)
            batch_size: Maximum users to sync per cycle (default: 10)
        """
        self.sleep_interval = sleep_interval
        self.batch_size = batch_size
        self.running = True
        self.db = db

        # Set up signal handlers for graceful shutdown
        signal.signal(signal.SIGTERM, self._handle_shutdown)
        signal.signal(signal.SIGINT, self._handle_shutdown)

        logger.info(
            f"UserSyncWorker initialized - "
            f"interval: {sleep_interval}s, batch_size: {batch_size}"
        )

    def _handle_shutdown(self, signum, frame):
        """Handle graceful shutdown on SIGTERM/SIGINT signals."""
        logger.info(f"Received signal {signum}, initiating graceful shutdown...")
        self.running = False

    def run(self):
        """
        Main worker loop.

        Continuously monitors the resource_users table for pending users
        and syncs them to their resources. Implements graceful shutdown handling.
        """
        logger.info("UserSyncWorker starting main loop")

        while self.running:
            try:
                self.sync_pending_users()
                time.sleep(self.sleep_interval)
            except KeyboardInterrupt:
                logger.info("KeyboardInterrupt received, shutting down")
                break
            except Exception as e:
                logger.error(
                    f"Unexpected error in worker loop: {e}",
                    exc_info=True
                )
                # Continue running despite errors
                time.sleep(self.sleep_interval)

        logger.info("UserSyncWorker shutdown complete")

    def sync_pending_users(self):
        """
        Sync all pending and updated users.

        Queries the resource_users table for:
        - sync_status='pending': Never synced users
        - Recently updated users that need re-syncing

        Processes up to batch_size users per cycle.
        """
        try:
            # Query for pending users
            pending_users = self.db(
                (self.db.resource_users.sync_status == 'pending') |
                (self.db.resource_users.sync_status == 'error')
            ).select(
                limitby=(0, self.batch_size),
                orderby=self.db.resource_users.created_at
            )

            if not pending_users:
                return

            logger.info(f"Found {len(pending_users)} pending users to sync")

            for resource_user in pending_users:
                if not self.running:
                    break

                try:
                    self.sync_user(resource_user.id)
                except Exception as e:
                    logger.error(
                        f"Error syncing user {resource_user.id}: {e}",
                        exc_info=True
                    )

        except Exception as e:
            logger.error(
                f"Error in sync_pending_users: {e}",
                exc_info=True
            )

    def sync_user(self, resource_user_id: int):
        """
        Sync a single user to its resource.

        Args:
            resource_user_id: ID of the resource_user record to sync

        Process:
        1. Load resource_user, resource, and resource_type
        2. Determine appropriate connector based on resource_type
        3. Initialize connector with connection info and credentials
        4. Call connector.create_user() or update_user()
        5. Update sync_status and timestamps on success
        6. On failure, update sync_status and sync_error
        """
        try:
            # Load resource_user record
            resource_user = self.db.resource_users[resource_user_id]
            if not resource_user:
                logger.warning(f"Resource user {resource_user_id} not found")
                return

            logger.info(
                f"Syncing user {resource_user.username} "
                f"(resource_user_id: {resource_user_id})"
            )

            # Update sync_status to 'syncing'
            resource_user.update_record(sync_status='syncing')
            self.db.commit()

            # Load resource
            resource = self.db.resources[resource_user.resource_id]
            if not resource:
                raise ValueError(
                    f"Resource {resource_user.resource_id} not found"
                )

            # Load resource_type
            resource_type = self.db.resource_types[resource.resource_type_id]
            if not resource_type:
                raise ValueError(
                    f"Resource type {resource.resource_type_id} not found"
                )

            # Get connector for this resource type
            connector = self._get_connector(
                resource_type.name,
                resource.connection_info,
                resource.credentials
            )

            if not connector:
                raise ValueError(
                    f"No connector available for resource type: {resource_type.name}"
                )

            # Sync user to resource
            user_data = {
                'username': resource_user.username,
                'password': resource_user.password_hash,
                'roles': resource_user.roles or [],
            }

            # Check if user already exists on resource
            user_exists = connector.user_exists(resource_user.username)

            if user_exists:
                logger.info(
                    f"Updating existing user {resource_user.username} "
                    f"on resource {resource.name}"
                )
                connector.update_user(resource_user.username, user_data)
            else:
                logger.info(
                    f"Creating new user {resource_user.username} "
                    f"on resource {resource.name}"
                )
                connector.create_user(user_data)

            # Mark as synced
            resource_user.update_record(
                sync_status='synced',
                last_synced_at=datetime.utcnow(),
                sync_error=None
            )
            self.db.commit()

            logger.info(
                f"Successfully synced user {resource_user.username} "
                f"to resource {resource.name}"
            )

        except ConnectionError as e:
            self._handle_sync_error(
                resource_user_id,
                f"Connection error: {str(e)}",
                "Connection failed - will retry"
            )
        except ValueError as e:
            self._handle_sync_error(
                resource_user_id,
                f"Configuration error: {str(e)}",
                "Invalid configuration - requires manual review"
            )
        except PermissionError as e:
            self._handle_sync_error(
                resource_user_id,
                f"Authentication error: {str(e)}",
                "Authentication failed - check credentials"
            )
        except Exception as e:
            self._handle_sync_error(
                resource_user_id,
                f"Sync failed: {str(e)}",
                "Unexpected error occurred - check logs"
            )

    def delete_user(self, resource_user_id: int):
        """
        Delete a user from its resource.

        Args:
            resource_user_id: ID of the resource_user record to delete

        Process:
        1. Load resource_user and verify sync_status is 'synced'
        2. Load resource and resource_type
        3. Initialize connector
        4. Call connector.delete_user()
        5. Remove resource_user record or mark as deleted
        """
        try:
            # Load resource_user record
            resource_user = self.db.resource_users[resource_user_id]
            if not resource_user:
                logger.warning(f"Resource user {resource_user_id} not found")
                return

            logger.info(
                f"Deleting user {resource_user.username} "
                f"(resource_user_id: {resource_user_id})"
            )

            # Load resource
            resource = self.db.resources[resource_user.resource_id]
            if not resource:
                raise ValueError(
                    f"Resource {resource_user.resource_id} not found"
                )

            # Load resource_type
            resource_type = self.db.resource_types[resource.resource_type_id]
            if not resource_type:
                raise ValueError(
                    f"Resource type {resource.resource_type_id} not found"
                )

            # Get connector for this resource type
            connector = self._get_connector(
                resource_type.name,
                resource.connection_info,
                resource.credentials
            )

            if not connector:
                raise ValueError(
                    f"No connector available for resource type: {resource_type.name}"
                )

            # Delete user from resource
            if connector.user_exists(resource_user.username):
                logger.info(
                    f"Deleting user {resource_user.username} "
                    f"from resource {resource.name}"
                )
                connector.delete_user(resource_user.username)
            else:
                logger.info(
                    f"User {resource_user.username} not found on resource, "
                    f"skipping deletion"
                )

            # Mark as deleted (soft delete)
            resource_user.update_record(
                deleted_at=datetime.utcnow(),
                sync_status='synced'
            )
            self.db.commit()

            logger.info(
                f"Successfully deleted user {resource_user.username} "
                f"from resource {resource.name}"
            )

        except Exception as e:
            logger.error(
                f"Error deleting user {resource_user_id}: {e}",
                exc_info=True
            )
            raise

    def _get_connector(
        self,
        resource_type_name: str,
        connection_info: Optional[Dict[str, Any]],
        credentials: Optional[Dict[str, Any]]
    ):
        """
        Get the appropriate connector for a resource type.

        Args:
            resource_type_name: Name of the resource type (e.g., 'db-postgresql')
            connection_info: Connection information (host, port, etc)
            credentials: Encrypted credentials (username, password, etc)

        Returns:
            Initialized connector instance or None if no connector available

        Supported resource types:
        - db-postgresql: PostgreSQL databases
        - db-mariadb: MariaDB databases
        - db-redis: Redis cache/data stores
        - db-valkey: Valkey (Redis-compatible) stores
        - storage-ceph: Ceph storage
        - storage-san: SAN storage
        """
        if not connection_info or not credentials:
            logger.error(
                f"Missing connection_info or credentials for {resource_type_name}"
            )
            return None

        try:
            if resource_type_name == 'db-postgresql':
                return PostgreSQLConnector(connection_info, credentials)

            elif resource_type_name == 'db-mariadb':
                return MariaDBConnector(connection_info, credentials)

            elif resource_type_name in ('db-redis', 'db-valkey'):
                return RedisConnector(connection_info, credentials)

            elif resource_type_name == 'storage-ceph':
                return CephConnector(connection_info, credentials)

            elif resource_type_name == 'storage-san':
                return SANConnector(connection_info, credentials)

            else:
                logger.error(
                    f"Unknown resource type: {resource_type_name}"
                )
                return None

        except Exception as e:
            logger.error(
                f"Error initializing connector for {resource_type_name}: {e}",
                exc_info=True
            )
            return None

    def _handle_sync_error(
        self,
        resource_user_id: int,
        error_message: str,
        user_message: str
    ):
        """
        Handle synchronization errors and update database.

        Args:
            resource_user_id: ID of the resource_user record
            error_message: Detailed error message for logging
            user_message: User-friendly message for sync_error field
        """
        try:
            resource_user = self.db.resource_users[resource_user_id]
            if resource_user:
                resource_user.update_record(
                    sync_status='error',
                    sync_error=user_message
                )
                self.db.commit()

                logger.error(
                    f"Sync error for user {resource_user.username}: "
                    f"{error_message}"
                )
        except Exception as e:
            logger.error(
                f"Error updating sync_error for user {resource_user_id}: {e}",
                exc_info=True
            )


def main():
    """
    Entry point for the UserSyncWorker.

    Reads configuration from environment variables:
    - SYNC_INTERVAL: Sleep interval between sync cycles (default: 30 seconds)
    - BATCH_SIZE: Maximum users to sync per cycle (default: 10)
    - LOG_LEVEL: Logging level (default: INFO)
    """
    # Get configuration from environment variables
    sync_interval = int(os.getenv('SYNC_INTERVAL', '30'))
    batch_size = int(os.getenv('BATCH_SIZE', '10'))
    log_level = os.getenv('LOG_LEVEL', 'INFO')

    # Configure logging level
    logging.getLogger().setLevel(log_level)

    logger.info("=" * 70)
    logger.info("NEST UserSyncWorker Starting")
    logger.info("=" * 70)
    logger.info(f"Configuration:")
    logger.info(f"  Sync Interval: {sync_interval}s")
    logger.info(f"  Batch Size: {batch_size}")
    logger.info(f"  Log Level: {log_level}")
    logger.info("=" * 70)

    # Create and run worker
    try:
        worker = UserSyncWorker(
            sleep_interval=sync_interval,
            batch_size=batch_size
        )
        worker.run()
    except Exception as e:
        logger.error(
            f"Fatal error in UserSyncWorker: {e}",
            exc_info=True
        )
        sys.exit(1)


if __name__ == '__main__':
    main()
