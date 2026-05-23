"""Tests for utils/sql_validator.py."""
import os
import sys
import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))


def _validate(sql: str):
    from utils.sql_validator import validate_sql_security
    return validate_sql_security(sql)


# ---------------------------------------------------------------------------
# Safe SQL
# ---------------------------------------------------------------------------

def test_safe_select():
    r = _validate("SELECT id, name FROM users WHERE id = 42")
    assert r.safe is True
    assert r.issues == []


def test_safe_with_condition():
    r = _validate("SELECT * FROM orders WHERE created_at > '2024-01-01'")
    assert r.safe is True


def test_safe_insert():
    r = _validate("INSERT INTO logs (user_id, action) VALUES (1, 'login')")
    assert r.safe is True


def test_safe_update_specific():
    r = _validate("UPDATE users SET email = 'x@x.com' WHERE id = 1")
    assert r.safe is True


def test_safe_delete_specific():
    r = _validate("DELETE FROM sessions WHERE expires_at < NOW()")
    assert r.safe is True


# ---------------------------------------------------------------------------
# Dangerous SQL patterns
# ---------------------------------------------------------------------------

def test_union_select():
    r = _validate("SELECT id FROM users UNION SELECT password FROM admins")
    assert r.safe is False
    assert any("UNION SELECT" in i for i in r.issues)


def test_exec_command():
    r = _validate("EXEC xp_cmdshell('dir')")
    assert r.safe is False
    assert any("EXEC" in i for i in r.issues)


def test_xp_cmdshell():
    r = _validate("EXEC xp_cmdshell('ls')")
    assert r.safe is False
    assert any("xp_cmdshell" in i for i in r.issues)


def test_bulk_insert():
    r = _validate("BULK INSERT table FROM 'file.csv'")
    assert r.safe is False


def test_drop_table():
    r = _validate("DROP TABLE users")
    assert r.safe is False
    assert any("DROP TABLE" in i for i in r.issues)


def test_drop_database():
    r = _validate("DROP DATABASE production")
    assert r.safe is False


def test_truncate_table():
    r = _validate("TRUNCATE TABLE sessions")
    assert r.safe is False


def test_alter_table():
    r = _validate("ALTER TABLE users ADD COLUMN hacked TEXT")
    assert r.safe is False


def test_waitfor():
    r = _validate("WAITFOR DELAY '0:0:5'")
    assert r.safe is False


def test_dbcc():
    r = _validate("DBCC CHECKDB")
    assert r.safe is False


def test_shutdown():
    r = _validate("SHUTDOWN")
    assert r.safe is False


def test_sql_comment_injection():
    r = _validate("SELECT 1 --")
    assert r.safe is False


def test_block_comment():
    r = _validate("SELECT /* comment */ 1")
    assert r.safe is False


def test_tautology():
    r = _validate("SELECT * FROM users WHERE 1=1")
    assert r.safe is False


def test_or_injection():
    r = _validate("SELECT * FROM users WHERE name='' or '1'='1'")
    assert r.safe is False


def test_sleep_function():
    r = _validate("SELECT SLEEP(5)")
    assert r.safe is False


def test_benchmark_function():
    r = _validate("SELECT BENCHMARK(1000000, MD5('test'))")
    assert r.safe is False


def test_information_schema():
    r = _validate("SELECT * FROM INFORMATION_SCHEMA.TABLES")
    assert r.safe is False


def test_openrowset():
    r = _validate("SELECT * FROM OPENROWSET('provider', 'datasource', 'query')")
    assert r.safe is False


def test_into_outfile():
    r = _validate("SELECT * INTO OUTFILE '/etc/passwd' FROM users")
    assert r.safe is False


def test_load_file():
    r = _validate("SELECT LOAD_FILE('/etc/passwd')")
    assert r.safe is False


def test_create_user():
    r = _validate("CREATE USER 'hacker'@'%' IDENTIFIED BY 'pass'")
    assert r.safe is False


def test_grant_all():
    r = _validate("GRANT ALL ON *.* TO 'hacker'@'%'")
    assert r.safe is False


def test_mass_update():
    r = _validate("UPDATE users SET password='hacked' WHERE 1=1")
    assert r.safe is False


def test_mass_delete():
    r = _validate("DELETE FROM users WHERE 1=1")
    assert r.safe is False


# ---------------------------------------------------------------------------
# Shell patterns
# ---------------------------------------------------------------------------

def test_bin_bash():
    r = _validate("SELECT '/bin/bash -c ls'")
    assert r.safe is False


def test_bin_sh():
    r = _validate("SELECT '/bin/sh'")
    assert r.safe is False


def test_powershell():
    r = _validate("SELECT 'powershell -c whoami'")
    assert r.safe is False


def test_system_call():
    r = _validate("system('ls')")
    assert r.safe is False


def test_popen_call():
    r = _validate("popen('ls')")
    assert r.safe is False


# ---------------------------------------------------------------------------
# Encoding attack detection
# ---------------------------------------------------------------------------

def test_non_ascii_characters():
    r = _validate("SELECT * FROM users WHERE name = 'héllo'")
    assert r.safe is False
    assert any("Non-ASCII" in i for i in r.issues)


def test_pure_ascii_is_safe():
    r = _validate("SELECT id FROM users WHERE active = true")
    assert r.safe is True


# ---------------------------------------------------------------------------
# ValidationResult dataclass
# ---------------------------------------------------------------------------

def test_validation_result_fields():
    from utils.sql_validator import ValidationResult
    vr = ValidationResult(safe=True, issues=[])
    assert vr.safe is True
    assert vr.issues == []


def test_validation_result_with_issues():
    from utils.sql_validator import ValidationResult
    vr = ValidationResult(safe=False, issues=["UNION SELECT detected"])
    assert vr.safe is False
    assert len(vr.issues) == 1


def test_validation_result_default_issues():
    from utils.sql_validator import ValidationResult
    vr = ValidationResult(safe=True)
    assert vr.issues == []


# ---------------------------------------------------------------------------
# Case insensitivity
# ---------------------------------------------------------------------------

def test_uppercase_patterns_detected():
    r = _validate("SELECT * FROM USERS UNION SELECT * FROM ADMINS")
    assert r.safe is False


def test_mixed_case_patterns_detected():
    r = _validate("Select * From Users uNiOn SeLeCt * From AdMiNs")
    assert r.safe is False
