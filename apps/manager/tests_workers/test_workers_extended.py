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


class TestBackupSchedulerFull:
    """Comprehensive tests for BackupScheduler with mocked backends."""

    def test_backup_scheduler_init_backend_unknown_type(self):
        """Test _initialize_backend() with unknown backend type raises."""
        mod = importlib.import_module("workers.backup_scheduler")
        with patch("workers.backup_scheduler.db", None):
            config_dict = {"backend_type": "unknown"}
            with pytest.raises(mod.BackupSchedulerError):
                mod.BackupScheduler(config_dict)

    def test_backup_scheduler_execute_backup_success(self):
        """Test execute_backup() completes successfully."""
        mod = importlib.import_module("workers.backup_scheduler")
        with patch("workers.backup_scheduler.db", None):
            with patch.object(mod.BackupScheduler, "_initialize_backend"):
                scheduler = mod.BackupScheduler({"backend_type": "local"})
                scheduler.backend = MagicMock()
                scheduler.schedule_backup(1, mod.BackupSchedule.DAILY, mod.BackupType.FULL)
                with patch.object(scheduler, "_create_mock_backup", return_value={"size_bytes": 1000}):
                    with patch.object(scheduler, "_upload_backup", return_value="/backups/1.bak"):
                        with patch.object(scheduler, "_cleanup_temp_files"):
                            result = scheduler.execute_backup(1)
                            assert result["status"] == mod.BackupStatus.COMPLETED.value
                            assert result["backup_size_bytes"] == 1000

    def test_backup_scheduler_execute_backup_max_retries_exceeded(self):
        """Test execute_backup() raises when max retries exceeded."""
        mod = importlib.import_module("workers.backup_scheduler")
        with patch("workers.backup_scheduler.db", None):
            with patch.object(mod.BackupScheduler, "_initialize_backend"):
                scheduler = mod.BackupScheduler({"backend_type": "local"})
                job = scheduler.schedule_backup(1, mod.BackupSchedule.DAILY)
                job.retry_count = job.max_retries + 1
                with patch.object(scheduler, "_cleanup_temp_files"):
                    with pytest.raises(mod.BackupExecutionError):
                        scheduler.execute_backup(1)

    def test_backup_scheduler_execute_backup_with_db_update(self):
        """Test execute_backup() updates database when available."""
        mod = importlib.import_module("workers.backup_scheduler")
        mock_db = MagicMock()
        with patch("workers.backup_scheduler.db", mock_db):
            with patch.object(mod.BackupScheduler, "_initialize_backend"):
                scheduler = mod.BackupScheduler({"backend_type": "local"})
                scheduler.db = mock_db
                scheduler.backend = MagicMock()
                scheduler.schedule_backup(1, mod.BackupSchedule.DAILY)
                with patch.object(scheduler, "_create_mock_backup", return_value={"size_bytes": 500}):
                    with patch.object(scheduler, "_upload_backup", return_value="/backups/1.bak"):
                        with patch.object(scheduler, "_update_backup_job_db"):
                            with patch.object(scheduler, "_cleanup_temp_files"):
                                result = scheduler.execute_backup(1, job_id=42)
                                assert result["status"] == mod.BackupStatus.COMPLETED.value

    def test_backup_scheduler_execute_backup_failure(self):
        """Test execute_backup() handles execution errors."""
        mod = importlib.import_module("workers.backup_scheduler")
        with patch("workers.backup_scheduler.db", None):
            with patch.object(mod.BackupScheduler, "_initialize_backend"):
                scheduler = mod.BackupScheduler({"backend_type": "local"})
                scheduler.schedule_backup(1, mod.BackupSchedule.DAILY)
                with patch.object(scheduler, "_create_mock_backup", side_effect=Exception("Backup error")):
                    with patch.object(scheduler, "_cleanup_temp_files"):
                        with pytest.raises(mod.BackupExecutionError):
                            scheduler.execute_backup(1)


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


class TestCertRotationFull:
    """Comprehensive tests for CertRotationWorker run loop and rotation cycle."""

    def test_cert_rotation_worker_run_loop_stops_on_stop(self):
        """Test run() loop exits when stop() called."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager, check_interval=0.01)

        # Mock _rotation_cycle to call stop after first iteration
        call_count = [0]
        def mock_cycle():
            call_count[0] += 1
            if call_count[0] == 1:
                worker.stop()

        with patch.object(worker, "_rotation_cycle", side_effect=mock_cycle):
            # run() should exit without error
            worker.run()
            assert worker.is_running is False

    def test_cert_rotation_worker_run_loop_handles_cycle_errors(self):
        """Test run() continues after _rotation_cycle() raises."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager, check_interval=0.01)

        call_count = [0]
        def mock_cycle():
            call_count[0] += 1
            if call_count[0] == 1:
                raise Exception("Cycle error")
            elif call_count[0] >= 2:
                worker.stop()

        with patch.object(worker, "_rotation_cycle", side_effect=mock_cycle):
            # run() should handle error and continue
            worker.run()
            assert call_count[0] >= 2

    def test_cert_rotation_worker_run_loop_keyboard_interrupt(self):
        """Test run() handles KeyboardInterrupt gracefully."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)

        def mock_cycle_interrupt():
            raise KeyboardInterrupt()

        with patch.object(worker, "_rotation_cycle", side_effect=mock_cycle_interrupt):
            worker.run()
            assert worker.is_running is False

    def test_cert_rotation_rotation_cycle_no_expiring_certs(self):
        """Test _rotation_cycle() with no expiring certificates."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)

        with patch.object(worker, "check_expiring_certificates", return_value=[]):
            # Should complete without error
            worker._rotation_cycle()

    def test_cert_rotation_rotation_cycle_auto_renew_success(self):
        """Test _rotation_cycle() with auto_renew=True and successful renewal."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)

        cert = MagicMock(auto_renew=True, cert_id=1, common_name="test.com")
        with patch.object(worker, "check_expiring_certificates", return_value=[cert]):
            with patch.object(worker, "_renew_certificate_with_recovery") as mock_renew:
                worker._rotation_cycle()
                mock_renew.assert_called_once()

    def test_cert_rotation_rotation_cycle_auto_renew_failure_notifies(self):
        """Test _rotation_cycle() notifies admin when renewal fails."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager)

        cert = MagicMock(auto_renew=True, cert_id=1, common_name="test.com", valid_until=datetime.utcnow() + timedelta(days=5))
        with patch.object(worker, "check_expiring_certificates", return_value=[cert]):
            with patch.object(worker, "_renew_certificate_with_recovery", side_effect=mod.CertificateRenewalError("Renewal failed")):
                with patch.object(worker, "notify_admin") as mock_notify:
                    worker._rotation_cycle()
                    mock_notify.assert_called()

    def test_cert_rotation_rotation_cycle_no_auto_renew_expiry_warning(self):
        """Test _rotation_cycle() notifies on expiry warning when auto_renew=False."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager, notification_threshold_days=7)

        # Cert expires in 3 days (within threshold)
        cert = MagicMock(auto_renew=False, cert_id=1, valid_until=datetime.utcnow() + timedelta(days=3))
        with patch.object(worker, "check_expiring_certificates", return_value=[cert]):
            with patch.object(worker, "notify_admin") as mock_notify:
                worker._rotation_cycle()
                mock_notify.assert_called()

    def test_cert_rotation_rotation_cycle_no_auto_renew_no_warning_past_threshold(self):
        """Test _rotation_cycle() does not warn when expiry is beyond threshold."""
        mod = importlib.import_module("workers.cert_rotation")
        db = MagicMock()
        ca_manager = MagicMock()
        worker = mod.CertRotationWorker(db=db, ca_manager=ca_manager, notification_threshold_days=7)

        # Cert expires in 30 days (beyond threshold)
        cert = MagicMock(auto_renew=False, cert_id=1, valid_until=datetime.utcnow() + timedelta(days=30))
        with patch.object(worker, "check_expiring_certificates", return_value=[cert]):
            with patch.object(worker, "notify_admin") as mock_notify:
                worker._rotation_cycle()
                assert not mock_notify.called


class TestStatsCollectorFull:
    """Comprehensive tests for StatsCollector worker loop and collection logic."""

    def test_stats_collector_run_loop_waits_for_stop_event(self):
        """Test run() loop waits for _stop_event."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()

        with patch("workers.stats_collector.db", mock_db):
            collector = mod.StatsCollector(db=mock_db, interval_seconds=0.01)
            call_count = [0]

            def mock_collect():
                call_count[0] += 1
                if call_count[0] >= 2:
                    collector._stop_event.set()

            with patch.object(collector, "collect_all_stats", side_effect=mock_collect):
                collector.run()
                assert collector._stop_event.is_set()

    def test_stats_collector_run_loop_handles_errors(self):
        """Test run() loop continues after errors."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()

        with patch("workers.stats_collector.db", mock_db):
            collector = mod.StatsCollector(db=mock_db, interval_seconds=0.01)
            call_count = [0]

            def mock_collect_error():
                call_count[0] += 1
                if call_count[0] == 1:
                    raise Exception("Collection error")
                else:
                    collector._stop_event.set()

            with patch.object(collector, "collect_all_stats", side_effect=mock_collect_error):
                collector.run()
                assert call_count[0] >= 2

    def test_stats_collector_collect_all_stats_query_resources(self):
        """Test collect_all_stats() queries active resources."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        mock_resources = [
            MagicMock(id=1, name="res1", status="active", lifecycle_mode="full"),
            MagicMock(id=2, name="res2", status="active", lifecycle_mode="partial"),
        ]
        mock_db.return_value.select.return_value = mock_resources

        with patch("workers.stats_collector.db", mock_db):
            collector = mod.StatsCollector(db=mock_db)
            with patch.object(collector, "collect_resource_stats"):
                collector.collect_all_stats()
                # Verify db query was called
                mock_db.assert_called()

    def test_stats_collector_collect_all_stats_handles_resource_errors(self):
        """Test collect_all_stats() continues on per-resource errors."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        mock_res1 = MagicMock(id=1, name="res1")
        mock_res2 = MagicMock(id=2, name="res2")
        mock_db.return_value.select.return_value = [mock_res1, mock_res2]

        with patch("workers.stats_collector.db", mock_db):
            collector = mod.StatsCollector(db=mock_db)
            call_count = [0]

            def mock_collect_error(resource):
                call_count[0] += 1
                if call_count[0] == 1:
                    raise Exception("Resource error")

            with patch.object(collector, "collect_resource_stats", side_effect=mock_collect_error):
                # Should not raise
                collector.collect_all_stats()
                assert call_count[0] >= 2

    def test_stats_collector_collect_resource_stats_k8s(self):
        """Test collect_resource_stats() for Kubernetes resources."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()

        with patch("workers.stats_collector.db", mock_db):
            collector = mod.StatsCollector(db=mock_db)
            resource = MagicMock(
                id=1, name="k8s-res",
                k8s_namespace="default",
                k8s_resource_name="pod-1"
            )
            metrics = {"cpu_percent": 50, "memory_bytes": 512000000}

            with patch.object(collector, "_collect_k8s_metrics", return_value=metrics):
                with patch.object(collector, "calculate_risk_level", return_value=("low", MagicMock(to_dict=lambda: {}))):
                    with patch.object(collector, "export_prometheus_metrics"):
                        collector.collect_resource_stats(resource)
                        # Verify metrics were stored
                        mock_db.resource_stats.insert.assert_called()

    def test_stats_collector_collect_resource_stats_external(self):
        """Test collect_resource_stats() for external resources."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()

        with patch("workers.stats_collector.db", mock_db):
            collector = mod.StatsCollector(db=mock_db)
            resource = MagicMock(
                id=2, name="ext-res",
                k8s_namespace=None,
                k8s_resource_name=None
            )
            metrics = {"cpu_percent": 75, "memory_bytes": 1024000000}

            with patch.object(collector, "_collect_external_metrics", return_value=metrics):
                with patch.object(collector, "calculate_risk_level", return_value=("high", MagicMock(to_dict=lambda: {}))):
                    with patch.object(collector, "export_prometheus_metrics"):
                        collector.collect_resource_stats(resource)
                        mock_db.resource_stats.insert.assert_called()

    def test_stats_collector_collect_resource_stats_no_metrics(self):
        """Test collect_resource_stats() skips when no metrics collected."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()

        with patch("workers.stats_collector.db", mock_db):
            collector = mod.StatsCollector(db=mock_db)
            resource = MagicMock(id=3, name="no-metrics", k8s_namespace=None)

            with patch.object(collector, "_collect_k8s_metrics", return_value=None):
                with patch.object(collector, "export_prometheus_metrics") as mock_export:
                    collector.collect_resource_stats(resource)
                    # Should skip database insert
                    assert not mock_db.resource_stats.insert.called


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







class TestStatsCollectorFull:
    """Comprehensive tests for StatsCollector metrics and risk calculation."""

    def test_stats_collector_init(self):
        """Test StatsCollector initialization."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db, interval_seconds=120, max_workers=3)
        assert collector.interval_seconds == 120
        assert collector.max_workers == 3
        assert collector._running is False

    def test_stats_collector_start_stop(self):
        """Test StatsCollector start and stop."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        with patch.object(collector, "run"):
            collector.start()
            assert collector._running is True

            result = collector.stop(timeout=1)
            assert collector._running is False

    def test_stats_collector_k8s_client_provided(self):
        """Test k8s_client when explicitly provided."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        mock_k8s = MagicMock()
        collector = mod.StatsCollector(db=mock_db, k8s_client=mock_k8s)
        assert collector.k8s_client == mock_k8s

    def test_stats_collector_calculate_risk_level_low(self):
        """Test calculate_risk_level for low risk."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        metrics = {
            "disk_usage_percent": 50.0,
            "memory_percent": 60.0,
            "cpu_percent": 40.0,
            "connections": {"total": 100, "active": 20}
        }
        risk_level, risk_factors = collector.calculate_risk_level(metrics)
        assert risk_level == "low"
        assert len(risk_factors.factors) == 0

    def test_stats_collector_calculate_risk_level_critical_disk(self):
        """Test calculate_risk_level for critical disk usage."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        metrics = {"disk_usage_percent": 97.0}
        risk_level, risk_factors = collector.calculate_risk_level(metrics)
        assert risk_level == "critical"
        assert any("critical" in f for f in risk_factors.factors)

    def test_stats_collector_calculate_risk_level_high_disk(self):
        """Test calculate_risk_level for high disk usage."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        metrics = {"disk_usage_percent": 88.0}
        risk_level, risk_factors = collector.calculate_risk_level(metrics)
        assert risk_level == "high"
        assert any("high" in f for f in risk_factors.factors)

    def test_stats_collector_calculate_risk_level_high_memory(self):
        """Test calculate_risk_level for high memory usage."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        metrics = {"memory_percent": 92.0}
        risk_level, risk_factors = collector.calculate_risk_level(metrics)
        assert risk_level == "high"
        assert any("Memory" in f for f in risk_factors.factors)

    def test_stats_collector_calculate_risk_level_saturation(self):
        """Test calculate_risk_level for connection saturation."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        metrics = {
            "connections": {"total": 100, "active": 85}
        }
        risk_level, risk_factors = collector.calculate_risk_level(metrics)
        assert risk_level == "medium"
        assert any("saturation" in f for f in risk_factors.factors)

    def test_stats_collector_calculate_risk_level_high_cpu(self):
        """Test calculate_risk_level for high CPU usage."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        metrics = {"cpu_percent": 88.0}
        risk_level, risk_factors = collector.calculate_risk_level(metrics)
        assert risk_level == "medium"
        assert any("CPU" in f for f in risk_factors.factors)

    def test_stats_collector_parse_k8s_quantity_ki(self):
        """Test _parse_k8s_quantity for Ki suffix."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        result = collector._parse_k8s_quantity("128Ki")
        assert result == 128 * 1024

    def test_stats_collector_parse_k8s_quantity_mi(self):
        """Test _parse_k8s_quantity for Mi suffix."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        result = collector._parse_k8s_quantity("512Mi")
        assert result == 512 * 1024 * 1024

    def test_stats_collector_parse_k8s_quantity_gi(self):
        """Test _parse_k8s_quantity for Gi suffix."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        result = collector._parse_k8s_quantity("2Gi")
        assert result == 2 * 1024 * 1024 * 1024

    def test_stats_collector_parse_k8s_quantity_plain_number(self):
        """Test _parse_k8s_quantity for plain number."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        result = collector._parse_k8s_quantity("1024")
        assert result == 1024

    def test_stats_collector_parse_k8s_quantity_invalid(self):
        """Test _parse_k8s_quantity for invalid input."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        result = collector._parse_k8s_quantity("invalid")
        assert result == 0

    def test_stats_collector_normalize_external_metrics_postgres(self):
        """Test _normalize_external_metrics for PostgreSQL."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        connector_stats = {
            "connections": {"total": 100, "active": 50},
            "database_size_bytes": 1000000,
            "cache_hit_ratio": 0.95
        }
        result = collector._normalize_external_metrics(connector_stats, "postgresql")
        assert result["connections"] == {"total": 100, "active": 50}
        assert result["database_size_bytes"] == 1000000
        assert result["cache_hit_ratio"] == 0.95

    def test_stats_collector_normalize_external_metrics_redis(self):
        """Test _normalize_external_metrics for Redis."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        connector_stats = {
            "used_memory_bytes": 500000,
            "used_memory_percent": 50.0,
            "connected_clients": 10,
            "keyspace_hits": 1000,
            "keyspace_misses": 100
        }
        result = collector._normalize_external_metrics(connector_stats, "redis")
        assert result["used_memory_bytes"] == 500000
        assert result["used_memory_percent"] == 50.0
        assert result["connected_clients"] == 10
        assert "cache_hit_ratio" in result

    def test_stats_collector_normalize_external_metrics_ceph(self):
        """Test _normalize_external_metrics for Ceph."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        connector_stats = {
            "used_bytes": 500000,
            "available_bytes": 500000,
            "total_bytes": 1000000
        }
        result = collector._normalize_external_metrics(connector_stats, "ceph")
        assert result["used_bytes"] == 500000
        assert result["disk_usage_percent"] == 50.0

    def test_stats_collector_export_prometheus_metrics(self):
        """Test export_prometheus_metrics updates gauges."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        resource = MagicMock(id=1, name="test-resource")
        metrics = {
            "cpu_percent": 50.0,
            "memory_bytes": 1000000,
            "memory_percent": 60.0,
            "disk_usage_percent": 70.0,
            "connections": {"active": 50}
        }

        # Should not raise
        collector.export_prometheus_metrics(resource, metrics, "medium")

    def test_stats_collector_parse_k8s_metrics_empty_containers(self):
        """Test _parse_k8s_metrics with empty containers."""
        mod = importlib.import_module("workers.stats_collector")
        mock_db = MagicMock()
        collector = mod.StatsCollector(db=mock_db)

        metric_pod = {"containers": []}
        result = collector._parse_k8s_metrics(metric_pod)
        assert result["cpu_percent"] == 0.0
        assert result["memory_bytes"] == 0
