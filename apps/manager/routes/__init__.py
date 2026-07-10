"""Route blueprints for NEST Manager API."""

from .analytics import analytics_bp
from .audit import audit_bp
from .auth import auth_bp
from .blocked_databases import blocked_bp
from .cloud import cloud_bp
from .database_servers import servers_bp
from .license import license_bp
from .managed_databases import databases_bp
from .permissions import permissions_bp
from .scaling import scaling_bp
from .security_rules import security_rules_bp
from .sql_files import sql_files_bp
from .stats import stats_bp
from .sync import sync_bp
from .teams import teams_bp
from .temporary_access import temp_access_bp
from .threat_intel import threat_intel_bp
from .user_profiles import profiles_bp

__all__ = [
    "analytics_bp",
    "audit_bp",
    "auth_bp",
    "blocked_bp",
    "cloud_bp",
    "servers_bp",
    "license_bp",
    "databases_bp",
    "permissions_bp",
    "scaling_bp",
    "security_rules_bp",
    "sql_files_bp",
    "stats_bp",
    "sync_bp",
    "teams_bp",
    "temp_access_bp",
    "threat_intel_bp",
    "profiles_bp",
]
