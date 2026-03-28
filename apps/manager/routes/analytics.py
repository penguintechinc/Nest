"""
Analytics routes — ported from apps/api/main.go (getAdvancedAnalytics /
getEnterpriseReports) with aggregate data pulled from PyDAL models.

Port notes:
- Go gated /advanced/analytics on "advanced_analytics" feature flag and
  /enterprise/reports on "enterprise_features" via license middleware.
  Here we map those gates to role checks (admin for enterprise reports,
  authenticated for advanced analytics) matching the Go handler behaviour
  of returning 200 for entitled callers without further DB logic.
  License feature-gating can be layered on top later via the licensing
  module once it is wired into the manager.
- Aggregate queries use PyDAL so all DB access remains in asyncio.to_thread.
"""

import asyncio
import logging
from datetime import datetime, timedelta, timezone

from quart import Blueprint, jsonify

from penguin_dal.quart_ext import get_db
from utils.auth import require_auth, require_role

logger = logging.getLogger(__name__)

analytics_bp = Blueprint("analytics", __name__, url_prefix="/api/v1")


# ---------------------------------------------------------------------------
# GET /api/v1/advanced/analytics
# ---------------------------------------------------------------------------

@analytics_bp.route("/advanced/analytics", methods=["GET"])
@require_auth
async def advanced_analytics() -> tuple:
    """Return aggregate analytics data.

    Requires authentication (any role).
    Data includes: resource counts by status, audit log activity over last 7 days,
    team counts, and user counts.
    """

    def _do_query() -> dict:

        db = get_db()
        # Resource counts by status
        resource_rows = db(db.resources.id > 0).select(
            db.resources.status,
            db.resources.id.count().with_alias("cnt"),
            groupby=db.resources.status,
        )
        resources_by_status = {r.resources.status: int(r.cnt) for r in resource_rows}
        total_resources = sum(resources_by_status.values())

        # Active resources (status == 'running' or 'active')
        active_resources = sum(
            v for k, v in resources_by_status.items()
            if k in ("running", "active", "ready")
        )

        # Audit log activity — last 7 days
        since = datetime.now(timezone.utc) - timedelta(days=7)
        audit_count = db(db.audit_logs.timestamp >= since).count()

        # Team count
        team_count = db(
            (db.teams.deleted_at == None)  # noqa: E711
        ).count()

        # User count (active)
        user_count = db(db.users.is_active == True).count()  # noqa: E712

        return {
            "message": "Advanced analytics data",
            "data": {
                "resources": {
                    "total": total_resources,
                    "active": active_resources,
                    "by_status": resources_by_status,
                },
                "audit_log": {
                    "events_last_7_days": int(audit_count),
                },
                "teams": {
                    "total": int(team_count),
                },
                "users": {
                    "active": int(user_count),
                },
            },
        }

    result = await asyncio.to_thread(_do_query)
    return jsonify(result), 200


# ---------------------------------------------------------------------------
# GET /api/v1/enterprise/reports
# ---------------------------------------------------------------------------

@analytics_bp.route("/enterprise/reports", methods=["GET"])
@require_role("admin")
async def enterprise_reports() -> tuple:
    """Return enterprise report data.

    Requires admin role (license-gated as enterprise feature).
    Provides security audit summary, compliance data, and usage analytics.
    """

    def _do_query() -> dict:

        db = get_db()
        # Security audit: blocked databases + security rules
        blocked_count = db(db.blocked_databases.id > 0).count()
        security_rule_count = db(db.security_rules.id > 0).count()

        # Certificates — expiring within 30 days
        in_30_days = datetime.now(timezone.utc) + timedelta(days=30)
        expiring_certs = db(
            (db.certificates.expires_at <= in_30_days)
            & (db.certificates.revoked_at == None)  # noqa: E711
        ).count()

        # Backup jobs — completed vs failed last 30 days
        since_30 = datetime.now(timezone.utc) - timedelta(days=30)
        completed_backups = db(
            (db.backup_jobs.status == "completed")
            & (db.backup_jobs.created_at >= since_30)
        ).count()
        failed_backups = db(
            (db.backup_jobs.status == "failed")
            & (db.backup_jobs.created_at >= since_30)
        ).count()

        # Provisioning jobs — last 30 days
        total_provisioning = db(
            db.provisioning_jobs.created_at >= since_30
        ).count()

        return {
            "message": "Enterprise reports",
            "reports": [
                "security_audit",
                "compliance_report",
                "usage_analytics",
            ],
            "data": {
                "security_audit": {
                    "blocked_databases": int(blocked_count),
                    "active_security_rules": int(security_rule_count),
                    "certificates_expiring_in_30_days": int(expiring_certs),
                },
                "compliance_report": {
                    "backup_jobs_completed_30d": int(completed_backups),
                    "backup_jobs_failed_30d": int(failed_backups),
                    "provisioning_jobs_30d": int(total_provisioning),
                },
                "usage_analytics": {
                    "total_resources": db(db.resources.id > 0).count(),
                    "total_teams": db(db.teams.deleted_at == None).count(),  # noqa: E711
                    "total_active_users": db(db.users.is_active == True).count(),  # noqa: E712
                },
            },
        }

    result = await asyncio.to_thread(_do_query)
    return jsonify(result), 200
