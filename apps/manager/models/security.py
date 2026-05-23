"""
Security model stubs.

Tables: security_rule, blocked_database
Schema: see apps/manager/migrations/

Tables are auto-reflected by penguin-dal — no define_table() needed.

Additional columns for audit_logs (e.g. threat_level, rule_id) are managed
exclusively via Alembic migrations. See apps/manager/migrations/ for the
current schema history.

Query helpers can be added here as needed.
"""
