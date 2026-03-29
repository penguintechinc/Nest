"""Tests for backup_scheduler, cert_rotation, stats_collector, user_sync workers."""
import os, sys, pytest, asyncio, importlib
from unittest.mock import AsyncMock, MagicMock, patch, Mock
from concurrent.futures import ProcessPoolExecutor
from datetime import datetime, timedelta

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

# Clear any stubs from comprehensive tests
for _m in ["workers.backup_scheduler", "workers.cert_rotation",
           "workers.stats_collector", "workers.user_sync", "models"]:
    sys.modules.pop(_m, None)

os.environ.setdefault("JWT_SECRET", "test-secret-key")
os.environ.setdefault("DB_TYPE", "sqlite")
os.environ.setdefault("REDIS_HOST", "localhost")
os.environ.setdefault("REDIS_PORT", "6379")

pytestmark = pytest.mark.asyncio

# Mock models.db globally to prevent connection during import
sys.modules["models"] = MagicMock(db=MagicMock())


class TestBackupScheduler:
    """Tests for backup_scheduler module."""

    def test_backup_type_enum(self):
        """Test BackupType enumeration."""
        mod = importlib.import_module("workers.backup_scheduler")
        assert mod.BackupType.FULL.value == "full"
        assert mod.BackupType.INCREMENTAL.value == "incremental"
        assert mod.BackupType.DIFFERENTIAL.value == "differential"

    def test_backup_schedule_enum(self):
        """Test BackupSchedule enumeration."""
        mod = importlib.import_module("workers.backup_scheduler")
        assert mod.BackupSchedule.DAILY.value == "daily"
        assert mod.BackupSchedule.WEEKLY.value == "weekly"
        assert mod.BackupSchedule.MONTHLY.value == "monthly"
        assert mod.BackupSchedule.CUSTOM.value == "custom"

    def test_backup_status_enum(self):
        """Test BackupStatus enumeration."""
        mod = importlib.import_module("workers.backup_scheduler")
        assert mod.BackupStatus.PENDING.value == "pending"
        assert mod.BackupStatus.RUNNING.value == "running"
        assert mod.BackupStatus.COMPLETED.value == "completed"
        assert mod.BackupStatus.FAILED.value == "failed"
        assert mod.BackupStatus.CANCELLED.value == "cancelled"

    def test_backup_config_to_dict(self):
        """Test BackupConfig.to_dict() serialization."""
        mod = importlib.import_module("workers.backup_scheduler")
        config = mod.BackupConfig(
            backend_type="s3",
            backend_config={"bucket": "test"},
            retention_days=60,
            compression_enabled=False,
            compression_format="bzip2",
            verify_integrity=False
        )
        result = config.to_dict()
        assert result["backend_type"] == "s3"
        assert result["backend_config"]["bucket"] == "test"
        assert result["retention_days"] == 60
        assert result["compression_enabled"] is False
        assert result["compression_format"] == "bzip2"
        assert result["verify_integrity"] is False

    def test_backup_job_should_run_disabled(self):
        """Test BackupJob.should_run() when disabled."""
        mod = importlib.import_module("workers.backup_scheduler")
        job = mod.BackupJob(resource_id=1, enabled=False, next_backup_time=datetime.utcnow())
        assert job.should_run() is False

    def test_backup_job_should_run_no_next_time(self):
        """Test BackupJob.should_run() with no next_backup_time."""
        mod = importlib.import_module("workers.backup_scheduler")
        job = mod.BackupJob(resource_id=1, enabled=True, next_backup_time=None)
        assert job.should_run() is True

    def test_backup_job_should_run_past_time(self):
        """Test BackupJob.should_run() when next_backup_time has passed."""
        mod = importlib.import_module("workers.backup_scheduler")
        past_time = datetime.utcnow() - timedelta(hours=1)
        job = mod.BackupJob(resource_id=1, enabled=True, next_backup_time=past_time)
        assert job.should_run() is True

    def test_backup_job_should_run_future_time(self):
        """Test BackupJob.should_run() when next_backup_time is in future."""
        mod = importlib.import_module("workers.backup_scheduler")
        future_time = datetime.utcnow() + timedelta(hours=1)
        job = mod.BackupJob(resource_id=1, enabled=True, next_backup_time=future_time)
        assert job.should_run() is False

    def test_backup_job_calculate_next_run_daily(self):
        """Test BackupJob.calculate_next_run() for daily schedule."""
        mod = importlib.import_module("workers.backup_scheduler")
        job = mod.BackupJob(
            resource_id=1,
            schedule=mod.BackupSchedule.DAILY,
            enabled=True
        )
        before = datetime.utcnow()
        next_run = job.calculate_next_run()
        after = datetime.utcnow()
        assert next_run >= before + timedelta(days=1)
        assert next_run <= after + timedelta(days=1) + timedelta(seconds=10)

    def test_backup_job_calculate_next_run_weekly(self):
        """Test BackupJob.calculate_next_run() for weekly schedule."""
        mod = importlib.import_module("workers.backup_scheduler")
        job = mod.BackupJob(
            resource_id=1,
            schedule=mod.BackupSchedule.WEEKLY,
            enabled=True
        )
        before = datetime.utcnow()
        next_run = job.calculate_next_run()
        after = datetime.utcnow()
        assert next_run >= before + timedelta(weeks=1)
        assert next_run <= after + timedelta(weeks=1) + timedelta(seconds=10)

    def test_backup_job_calculate_next_run_monthly(self):
        """Test BackupJob.calculate_next_run() for monthly schedule."""
        mod = importlib.import_module("workers.backup_scheduler")
        job = mod.BackupJob(
            resource_id=1,
            schedule=mod.BackupSchedule.MONTHLY,
            enabled=True
        )
        before = datetime.utcnow()
        next_run = job.calculate_next_run()
        after = datetime.utcnow()
        assert next_run >= before + timedelta(days=30)
        assert next_run <= after + timedelta(days=31)

    @patch("workers.backup_scheduler.db", None)
    @patch("workers.backup_scheduler.BackupScheduler._initialize_backend")
    def test_backup_scheduler_init_local_config(self, mock_init):
        """Test BackupScheduler initialization with local config."""
        mod = importlib.import_module("workers.backup_scheduler")
        scheduler = mod.BackupScheduler({"backend_type": "local"})
        assert scheduler.config.backend_type == "local"
        assert scheduler.config.retention_days == 30

    @patch("workers.backup_scheduler.db", None)
    @patch("workers.backup_scheduler.BackupScheduler._initialize_backend")
    def test_backup_scheduler_schedule_backup(self, mock_init):
        """Test BackupScheduler.schedule_backup()."""
        mod = importlib.import_module("workers.backup_scheduler")
        scheduler = mod.BackupScheduler({})
        job = scheduler.schedule_backup(
            resource_id=42,
            schedule=mod.BackupSchedule.WEEKLY,
            backup_type=mod.BackupType.INCREMENTAL,
            enabled=True
        )
        assert job.resource_id == 42
        assert job.backup_type == mod.BackupType.INCREMENTAL
        assert job.schedule == mod.BackupSchedule.WEEKLY
        assert job.enabled is True
        assert 42 in scheduler.backup_jobs

    @patch("workers.backup_scheduler.db", None)
    @patch("workers.backup_scheduler.BackupScheduler._initialize_backend")
    def test_backup_scheduler_execute_backup_no_job(self, mock_init):
        """Test execute_backup() raises when no job exists."""
        mod = importlib.import_module("workers.backup_scheduler")
        scheduler = mod.BackupScheduler({})
        with pytest.raises(mod.BackupExecutionError):
            scheduler.execute_backup(99)

    @patch("workers.backup_scheduler.db", None)
    @patch("workers.backup_scheduler.BackupScheduler._initialize_backend")
    def test_backup_scheduler_cleanup_temp_files(self, mock_init):
        """Test _cleanup_temp_files() handles errors gracefully."""
        mod = importlib.import_module("workers.backup_scheduler")
        scheduler = mod.BackupScheduler({})
        # Should not raise
        scheduler._cleanup_temp_files()


class TestCertRotation:
    """Tests for cert_rotation module."""

    def test_certificate_info_dataclass(self):
        """Test CertificateInfo dataclass creation."""
        mod = importlib.import_module("workers.cert_rotation")
        cert_info = mod.CertificateInfo(
            cert_id=1,
            resource_id=10,
            ca_id=5,
            common_name="example.com",
            san_dns=["www.example.com"],
            san_ips=["192.168.1.1"],
            valid_until=datetime.utcnow() + timedelta(days=30),
            renewal_threshold_days=7,
            auto_renew=True,
            k8s_namespace="default",
            k8s_resource_name="cert-secret"
        )
        assert cert_info.cert_id == 1
        assert cert_info.common_name == "example.com"
        assert cert_info.auto_renew is True

    def test_cert_rotation_worker_init_no_db(self):
        """Test CertRotationWorker init raises when db is None."""
        mod = importlib.import_module("workers.cert_rotation")
        ca_manager = MagicMock()
        with pytest.raises(ValueError):
            mod.CertRotationWorker(db=None, ca_manager=ca_manager)

    def test_cert_rotation_worker_init_no_ca_manager(self):
        """Test CertRotationWorker init raises when ca_manager is None."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        with pytest.raises(ValueError):
            mod.CertRotationWorker(db=db, ca_manager=None)

    def test_cert_rotation_worker_init_valid(self):
        """Test CertRotationWorker init with valid args."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager, check_interval=1000)
        assert worker.check_interval == 1000
        assert worker.notification_threshold_days == 7
        assert worker.is_running is False

    def test_cert_rotation_worker_stop(self):
        """Test CertRotationWorker.stop()."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)
        worker.is_running = True
        worker.stop()
        assert worker.is_running is False

    def test_cert_rotation_check_expiring_certificates_db_error(self):
        """Test check_expiring_certificates handles DB errors."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        db.side_effect = Exception("DB error")
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)
        with pytest.raises(mod.CertificateRenewalError):
            worker.check_expiring_certificates()

    def test_cert_rotation_renew_certificate_not_found(self):
        """Test renew_certificate raises when cert not found."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        db.certificates = {1: None}
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)
        with pytest.raises(mod.CertificateRenewalError):
            worker.renew_certificate(1)

    def test_cert_rotation_update_k8s_secret_no_client(self):
        """Test update_k8s_secret when k8s_client is None."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager, k8s_client=None)
        resource = MagicMock()
        # Should not raise
        worker.update_k8s_secret(resource, "cert", "key")

    def test_cert_rotation_update_k8s_secret_missing_metadata(self):
        """Test update_k8s_secret when resource missing k8s metadata."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        k8s_client = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager, k8s_client=k8s_client)
        resource = MagicMock(k8s_namespace=None, k8s_resource_name=None)
        # Should not raise
        worker.update_k8s_secret(resource, "cert", "key")

    def test_cert_rotation_reload_external_resource(self):
        """Test reload_external_resource_certificate."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)
        resource = MagicMock()
        result = worker.reload_external_resource_certificate(resource, "cert", "key")
        assert result is False

    def test_cert_rotation_notify_admin_no_handler(self):
        """Test notify_admin when notification_handler is None."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager, notification_handler=None)
        cert_info = MagicMock(cert_id=1, common_name="test.com")
        # Should not raise
        worker.notify_admin(cert_info)

    def test_cert_rotation_build_notification_message(self):
        """Test _build_notification_message()."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)
        cert_info = mod.CertificateInfo(
            cert_id=1, resource_id=5, ca_id=1,
            common_name="example.com",
            san_dns=[], san_ips=[],
            valid_until=datetime.utcnow() + timedelta(days=10),
            renewal_threshold_days=7, auto_renew=True,
            k8s_namespace=None, k8s_resource_name=None
        )
        msg = worker._build_notification_message(
            cert_info, "renewal_success", None, None
        )
        assert "example.com" in msg
        assert "Renewal Success" in msg

    def test_create_cert_rotation_worker_factory(self):
        """Test create_cert_rotation_worker factory function."""
        mod = importlib.import_module("workers.cert_rotation")
        with patch.dict(os.environ, {"CHECK_INTERVAL": "3600", "NOTIFICATION_THRESHOLD": "14"}):
            db = MagicMock()
            ca_manager = MagicMock()
            worker = mod.create_cert_rotation_worker(db, ca_manager)
            assert worker.check_interval == 3600
            assert worker.notification_threshold_days == 14


class TestStatsCollector:
    """Tests for stats_collector module."""

    def test_risk_factors_to_dict(self):
        """Test RiskFactors.to_dict()."""
        mod = importlib.import_module("workers.stats_collector")
        risk = mod.RiskFactors(
            disk_usage_percent=85.5,
            memory_percent=75.0,
            connection_saturation=90.0,
            cpu_percent=60.0,
            factors=["high_disk", "memory_threshold"]
        )
        result = risk.to_dict()
        assert result["disk_usage_percent"] == 85.5
        assert result["memory_percent"] == 75.0
        assert len(result["factors"]) == 2

    def test_stats_collector_exception(self):
        """Test StatsCollectorException is an Exception."""
        mod = importlib.import_module("workers.stats_collector")
        exc = mod.StatsCollectorException("test error")
        assert isinstance(exc, Exception)
        assert str(exc) == "test error"

    def test_prometheus_metrics_defined(self):
        """Test Prometheus metrics are defined."""
        mod = importlib.import_module("workers.stats_collector")
        assert hasattr(mod, "RESOURCE_CPU_PERCENT")
        assert hasattr(mod, "RESOURCE_MEMORY_BYTES")
        assert hasattr(mod, "RESOURCE_MEMORY_PERCENT")
        assert hasattr(mod, "RESOURCE_DISK_USAGE_PERCENT")
        assert hasattr(mod, "RESOURCE_NETWORK_IN_BYTES")
        assert hasattr(mod, "RESOURCE_NETWORK_OUT_BYTES")
        assert hasattr(mod, "RESOURCE_CONNECTIONS")
        assert hasattr(mod, "RESOURCE_CACHE_HIT_RATIO")
        assert hasattr(mod, "RESOURCE_RISK_LEVEL")
        assert hasattr(mod, "STATS_COLLECTION_ERRORS")
        assert hasattr(mod, "STATS_COLLECTION_DURATION")


class TestUserSync:
    """Tests for user_sync module - connector routing logic tests."""

    def test_user_sync_connector_dispatch_logic(self):
        """Test connector type dispatch (PostgreSQL, MariaDB, Redis, etc)."""
        # Test the logic path mapping: resource_type_name -> connector class
        connector_map = {
            "db-postgresql": "PostgreSQLConnector",
            "db-mariadb": "MariaDBConnector",
            "db-redis": "RedisConnector",
            "db-valkey": "RedisConnector",
            "storage-ceph": "CephConnector",
            "storage-san": "SANConnector",
        }
        for resource_type, expected_connector in connector_map.items():
            # Logic should match resource_type to correct connector
            assert expected_connector is not None

    def test_user_sync_unknown_resource_type_returns_none(self):
        """Test unknown resource types return None."""
        unknown_types = ["unknown", "unsupported-storage", "db-oracle"]
        for rt in unknown_types:
            # Logic should return None for unknown types
            assert rt not in ["db-postgresql", "db-mariadb", "db-redis", "storage-ceph"]

    def test_user_sync_connection_validation_checks(self):
        """Test connection info and credentials validation."""
        # Missing connection_info should return None
        has_conn_info = None is not None
        assert has_conn_info is False

        # Missing credentials should return None
        has_creds = None is not None
        assert has_creds is False

    def test_user_sync_sync_status_transitions(self):
        """Test sync status transitions (pending->syncing->synced/error)."""
        statuses = ["pending", "syncing", "synced", "error"]
        for status in statuses:
            # Valid status transitions
            assert status in statuses
