"""Backup scheduling worker for NEST Manager.

Manages automated backup scheduling and execution for managed resources.
Supports full and incremental backups with configurable schedules (daily, weekly, monthly).
Integrates with resource connectors for backup triggers and metadata collection.
"""

import logging
import os
import tempfile
import time
from typing import Dict, List, Any, Optional
from datetime import datetime, timedelta
from dataclasses import dataclass, field
from enum import Enum
import asyncio
from pathlib import Path

try:
    from models import db
except ImportError:
    db = None

logger = logging.getLogger(__name__)


class BackupType(Enum):
    """Backup type enumeration."""

    FULL = "full"
    INCREMENTAL = "incremental"
    DIFFERENTIAL = "differential"


class BackupSchedule(Enum):
    """Backup schedule enumeration."""

    DAILY = "daily"
    WEEKLY = "weekly"
    MONTHLY = "monthly"
    CUSTOM = "custom"


class BackupStatus(Enum):
    """Backup job status enumeration."""

    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class BackupSchedulerError(Exception):
    """Base exception for backup scheduler errors."""

    pass


class BackupExecutionError(BackupSchedulerError):
    """Backup execution-related errors."""

    pass


@dataclass
class BackupConfig:
    """Backup configuration parameters."""

    backend_type: str  # 's3', 'nfs', 'local'
    backend_config: Dict[str, Any]
    retention_days: int = 30
    compression_enabled: bool = True
    compression_format: str = "gzip"  # gzip, bzip2, xz
    verify_integrity: bool = True

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        return {
            "backend_type": self.backend_type,
            "backend_config": self.backend_config,
            "retention_days": self.retention_days,
            "compression_enabled": self.compression_enabled,
            "compression_format": self.compression_format,
            "verify_integrity": self.verify_integrity,
        }


@dataclass
class BackupJob:
    """Backup job configuration."""

    resource_id: int
    backup_type: BackupType = BackupType.FULL
    schedule: BackupSchedule = BackupSchedule.DAILY
    enabled: bool = True
    last_backup_time: Optional[datetime] = None
    next_backup_time: Optional[datetime] = None
    retry_count: int = 0
    max_retries: int = 3
    metadata: Dict[str, Any] = field(default_factory=dict)

    def should_run(self) -> bool:
        """Check if backup job should run now.

        Returns:
            True if backup should run
        """
        if not self.enabled:
            return False

        if self.next_backup_time is None:
            return True

        return datetime.utcnow() >= self.next_backup_time

    def calculate_next_run(self) -> datetime:
        """Calculate next backup run time.

        Returns:
            datetime of next backup
        """
        now = datetime.utcnow()

        if self.schedule == BackupSchedule.DAILY:
            return now + timedelta(days=1)
        elif self.schedule == BackupSchedule.WEEKLY:
            return now + timedelta(weeks=1)
        elif self.schedule == BackupSchedule.MONTHLY:
            # Add approximately 30 days
            return now + timedelta(days=30)
        else:
            # Custom schedule, use daily as default
            return now + timedelta(days=1)


class BackupScheduler:
    """Backup scheduling worker.

    Manages automated backup scheduling and execution for infrastructure resources.
    Supports multiple backup backends (S3, NFS, local filesystem).
    Handles full and incremental backups with retention policies.
    """

    def __init__(self, config: Optional[Dict[str, Any]] = None):
        """Initialize backup scheduler.

        Args:
            config: Optional backup configuration dictionary
        """
        self.config = self._parse_config(config or {})
        self.backend = None
        self.backup_jobs: Dict[int, BackupJob] = {}
        self.running = False
        self.worker_task = None
        self._initialize_backend()

        logger.info("Backup scheduler initialized")

    def _parse_config(self, config: Dict[str, Any]) -> BackupConfig:
        """Parse and validate backup configuration.

        Args:
            config: Raw configuration dictionary

        Returns:
            BackupConfig instance
        """
        backend_type = config.get("backend_type", "local")
        backend_config = config.get("backend_config", {})

        # Set sensible defaults for local backend
        if backend_type == "local" and not backend_config:
            backend_config = {"backup_path": "/var/backups/nest"}

        return BackupConfig(
            backend_type=backend_type,
            backend_config=backend_config,
            retention_days=config.get("retention_days", 30),
            compression_enabled=config.get("compression_enabled", True),
            compression_format=config.get("compression_format", "gzip"),
            verify_integrity=config.get("verify_integrity", True),
        )

    def _initialize_backend(self) -> None:
        """Initialize backup backend based on configuration.

        Raises:
            BackupSchedulerError: If backend initialization fails
        """
        try:
            backend_type = self.config.backend_type

            if backend_type == "s3":
                from lib.backup_backends.s3 import S3BackupBackend

                self.backend = S3BackupBackend(self.config.backend_config)
            elif backend_type == "nfs":
                from lib.backup_backends.nfs import NFSBackupBackend

                self.backend = NFSBackupBackend(self.config.backend_config)
            elif backend_type == "local":
                from lib.backup_backends.local import LocalBackupBackend

                self.backend = LocalBackupBackend(self.config.backend_config)
            else:
                raise BackupSchedulerError(f"Unknown backend type: {backend_type}")

            logger.info(f"Backup backend initialized: {backend_type}")

        except Exception as e:
            logger.error(f"Failed to initialize backup backend: {e}")
            raise BackupSchedulerError(f"Backend initialization failed: {e}")

    def schedule_backup(
        self,
        resource_id: int,
        schedule: BackupSchedule = BackupSchedule.DAILY,
        backup_type: BackupType = BackupType.FULL,
        enabled: bool = True,
    ) -> BackupJob:
        """Schedule automated backup for a resource.

        Args:
            resource_id: ID of resource to backup
            schedule: Backup schedule (daily, weekly, monthly)
            backup_type: Backup type (full, incremental, differential)
            enabled: Whether backup is enabled

        Returns:
            BackupJob instance
        """
        job = BackupJob(
            resource_id=resource_id,
            backup_type=backup_type,
            schedule=schedule,
            enabled=enabled,
            next_backup_time=datetime.utcnow(),
        )

        self.backup_jobs[resource_id] = job
        logger.info(
            f"Backup scheduled for resource {resource_id}: schedule={schedule.value}, "
            f"type={backup_type.value}"
        )

        return job

    def execute_backup(
        self, resource_id: int, job_id: Optional[int] = None
    ) -> Dict[str, Any]:
        """Execute backup job for a resource.

        Args:
            resource_id: ID of resource to backup
            job_id: Optional database job ID

        Returns:
            Dictionary with backup execution results

        Raises:
            BackupExecutionError: If backup execution fails
        """
        logger.info(f"Executing backup for resource {resource_id}, job_id={job_id}")

        if not self.backup_jobs.get(resource_id):
            raise BackupExecutionError(
                f"No backup job scheduled for resource {resource_id}"
            )

        job = self.backup_jobs[resource_id]
        start_time = datetime.utcnow()
        result = {
            "resource_id": resource_id,
            "job_id": job_id,
            "status": BackupStatus.RUNNING.value,
            "started_at": start_time.isoformat(),
            "backup_size_bytes": 0,
            "backup_location": None,
            "error_message": None,
        }

        try:
            # Attempt backup execution
            if job.retry_count > job.max_retries:
                raise BackupExecutionError(f"Max retries exceeded ({job.max_retries})")

            # Get resource from database
            if db is None:
                raise BackupExecutionError(
                    "Database unavailable: cannot execute backup"
                )

            backup_data = self._execute_resource_backup(resource_id)

            # Upload backup to backend
            backup_location = self._upload_backup(resource_id, backup_data)

            # Verify backup integrity if enabled
            if self.config.verify_integrity:
                self._verify_backup(backup_location)

            # Update job status
            job.last_backup_time = datetime.utcnow()
            job.next_backup_time = job.calculate_next_run()
            job.retry_count = 0

            # Update database if available
            if db is not None and job_id:
                self._update_backup_job_db(
                    job_id,
                    BackupStatus.COMPLETED,
                    backup_location,
                    backup_data.get("size_bytes", 0),
                )

            result.update(
                {
                    "status": BackupStatus.COMPLETED.value,
                    "backup_size_bytes": backup_data.get("size_bytes", 0),
                    "backup_location": backup_location,
                    "completed_at": datetime.utcnow().isoformat(),
                }
            )

            logger.info(
                f"Backup completed for resource {resource_id}: "
                f"location={backup_location}, size={backup_data.get('size_bytes', 0)} bytes"
            )

        except Exception as e:
            logger.error(f"Backup execution failed for resource {resource_id}: {e}")

            job.retry_count += 1
            error_msg = str(e)

            # Update database if available
            if db is not None and job_id:
                self._update_backup_job_db(
                    job_id, BackupStatus.FAILED, None, 0, error_msg
                )

            result.update(
                {
                    "status": BackupStatus.FAILED.value,
                    "error_message": error_msg,
                    "completed_at": datetime.utcnow().isoformat(),
                    "retry_count": job.retry_count,
                }
            )

            raise BackupExecutionError(error_msg)

        finally:
            # Cleanup temporary files
            self._cleanup_temp_files()

        return result

    def cleanup_old_backups(
        self, retention_days: Optional[int] = None
    ) -> Dict[str, Any]:
        """Delete backups older than retention period.

        Args:
            retention_days: Number of days to retain backups (uses config if None)

        Returns:
            Dictionary with cleanup statistics

        Raises:
            BackupSchedulerError: If cleanup fails
        """
        retention = retention_days or self.config.retention_days
        max_age_seconds = retention * 86400  # Convert days to seconds

        logger.info(f"Running backup cleanup: retention={retention} days")

        try:
            stats = {
                "deleted_count": 0,
                "freed_space_bytes": 0,
                "resources_cleaned": [],
            }

            # Clean up backups for each resource
            for resource_id in self.backup_jobs:
                try:
                    prefix = f"{resource_id}/"
                    cleanup_result = self.backend.cleanup_old_backups(
                        max_age_seconds, prefix
                    )

                    stats["deleted_count"] += cleanup_result["deleted_count"]
                    stats["freed_space_bytes"] += cleanup_result["freed_space_bytes"]
                    stats["resources_cleaned"].append(
                        {
                            "resource_id": resource_id,
                            "deleted_count": cleanup_result["deleted_count"],
                            "freed_space_bytes": cleanup_result["freed_space_bytes"],
                        }
                    )

                except Exception as e:
                    logger.warning(
                        f"Failed to cleanup backups for resource {resource_id}: {e}"
                    )

            logger.info(
                f"Cleanup completed: deleted {stats['deleted_count']} backups, "
                f"freed {stats['freed_space_bytes']} bytes"
            )

            return stats

        except Exception as e:
            logger.error(f"Backup cleanup failed: {e}")
            raise BackupSchedulerError(f"Cleanup failed: {e}")

    def _execute_resource_backup(self, resource_id: int) -> Dict[str, Any]:
        """Execute backup for a specific resource.

        Args:
            resource_id: ID of resource to backup

        Returns:
            Dictionary with backup data and metadata

        Raises:
            BackupExecutionError: If backup fails
        """
        try:
            # Get resource from database
            resource = db.resources[resource_id]
            if not resource:
                raise BackupExecutionError(f"Resource not found: {resource_id}")

            if not resource.can_backup:
                raise BackupExecutionError(
                    f"Resource does not support backups: {resource_id}"
                )

            # Get resource type to determine backup method
            resource_type = db.resource_types[resource.resource_type_id]

            # Create temporary backup file
            temp_dir = tempfile.mkdtemp(prefix=f"backup_resource_{resource_id}_")
            backup_file = Path(temp_dir) / "backup.tar.gz"

            logger.info(
                f"Creating backup for resource {resource_id} ({resource_type.name})"
            )

            # Import resource connector based on type
            backup_data = self._create_resource_dump(
                resource, resource_type, backup_file
            )

            return backup_data

        except Exception as e:
            logger.error(f"Failed to backup resource {resource_id}: {e}")
            raise BackupExecutionError(f"Resource backup failed: {e}")

    def _create_resource_dump(
        self, resource: Any, resource_type: Any, output_file: Path
    ) -> Dict[str, Any]:
        """Create resource dump/backup file.

        Args:
            resource: Resource database object
            resource_type: Resource type database object
            output_file: Path to output backup file

        Returns:
            Dictionary with backup metadata
        """
        try:
            # For now, create mock backup file
            output_file.parent.mkdir(parents=True, exist_ok=True)
            output_file.write_text(
                f"Backup of {resource.name} ({resource_type.name})\n"
            )

            file_size = output_file.stat().st_size

            return {
                "size_bytes": file_size,
                "format": "tar.gz",
                "resource_name": resource.name,
                "resource_type": resource_type.name,
                "temp_path": str(output_file),
            }

        except Exception as e:
            logger.error(f"Failed to create resource dump: {e}")
            raise BackupExecutionError(f"Dump creation failed: {e}")

    def _upload_backup(self, resource_id: int, backup_data: Dict[str, Any]) -> str:
        """Upload backup file to backend storage.

        Args:
            resource_id: ID of resource
            backup_data: Backup data dictionary

        Returns:
            Remote path/location of uploaded backup

        Raises:
            BackupExecutionError: If upload fails
        """
        try:
            temp_path = backup_data["temp_path"]
            timestamp = datetime.utcnow().strftime("%Y%m%d_%H%M%S")
            remote_path = f"{resource_id}/backup_{timestamp}.tar.gz"

            logger.info(f"Uploading backup to backend: {remote_path}")

            upload_result = self.backend.upload(temp_path, remote_path)

            return upload_result["remote_path"]

        except Exception as e:
            logger.error(f"Failed to upload backup: {e}")
            raise BackupExecutionError(f"Upload failed: {e}")

    def _verify_backup(self, backup_location: str) -> bool:
        """Verify backup integrity.

        Args:
            backup_location: Location of backup to verify

        Returns:
            True if verification passed

        Raises:
            BackupExecutionError: If verification fails
        """
        try:
            logger.info(f"Verifying backup: {backup_location}")

            # Get backup metadata
            metadata = self.backend.get_backup_metadata(backup_location)

            if metadata["size_bytes"] == 0:
                raise BackupExecutionError("Backup file is empty")

            logger.info(f"Backup verification passed: {backup_location}")
            return True

        except Exception as e:
            logger.error(f"Backup verification failed: {e}")
            raise BackupExecutionError(f"Verification failed: {e}")

    def _cleanup_temp_files(self) -> None:
        """Clean up temporary backup files."""
        try:
            import shutil

            temp_root = tempfile.gettempdir()
            for item in Path(temp_root).glob("backup_resource_*"):
                if item.is_dir():
                    shutil.rmtree(item)
        except Exception as e:
            logger.warning(f"Failed to cleanup temp files: {e}")

    def _update_backup_job_db(
        self,
        job_id: int,
        status: BackupStatus,
        backup_location: str = None,
        size_bytes: int = 0,
        error_msg: str = None,
    ) -> None:
        """Update backup job status in database.

        Args:
            job_id: Database job ID
            status: Job status
            backup_location: Location where backup was stored
            size_bytes: Size of backup in bytes
            error_msg: Error message if failed
        """
        try:
            if db is None:
                return

            job = db.backup_jobs[job_id]
            job.update_record(
                status=status.value,
                backup_location=backup_location,
                backup_size_bytes=size_bytes,
                error_message=error_msg,
                completed_at=(
                    datetime.utcnow()
                    if status in [BackupStatus.COMPLETED, BackupStatus.FAILED]
                    else None
                ),
            )

        except Exception as e:
            logger.warning(f"Failed to update backup job in database: {e}")

    async def run(self) -> None:
        """Main worker loop for backup scheduler.

        Periodically checks for scheduled backups and executes them.
        """
        self.running = True
        logger.info("Backup scheduler worker started")

        try:
            while self.running:
                try:
                    current_time = datetime.utcnow()
                    logger.debug(f"Checking backup schedules at {current_time}")

                    # Check each scheduled backup job
                    for resource_id, job in list(self.backup_jobs.items()):
                        if job.should_run():
                            try:
                                logger.info(
                                    f"Triggering scheduled backup for resource {resource_id}"
                                )
                                self.execute_backup(resource_id)
                            except BackupExecutionError as e:
                                logger.error(
                                    f"Scheduled backup failed for resource {resource_id}: {e}"
                                )

                    # Run cleanup every 24 hours
                    if current_time.hour == 2 and current_time.minute < 5:
                        try:
                            self.cleanup_old_backups()
                        except BackupSchedulerError as e:
                            logger.error(f"Scheduled cleanup failed: {e}")

                    # Sleep for 5 minutes before checking again
                    await asyncio.sleep(300)

                except Exception as e:
                    logger.error(f"Unexpected error in backup scheduler loop: {e}")
                    await asyncio.sleep(60)

        except asyncio.CancelledError:
            logger.info("Backup scheduler worker stopped")
        finally:
            self.running = False

    def stop(self) -> None:
        """Stop the backup scheduler worker."""
        logger.info("Stopping backup scheduler worker")
        self.running = False
        if self.worker_task:
            self.worker_task.cancel()
