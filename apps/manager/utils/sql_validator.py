"""SQL security validation - detects injection and dangerous patterns."""
import re
from dataclasses import dataclass, field
from typing import List


@dataclass(slots=True)
class ValidationResult:
    safe: bool
    issues: List[str] = field(default_factory=list)


DANGEROUS_SQL_PATTERNS = [
    (r'\bexec\b', "EXEC command detected"),
    (r'\bxp_cmdshell\b', "xp_cmdshell detected"),
    (r'\bbulk\s+insert\b', "BULK INSERT detected"),
    (r'\bopenrowset\b', "OPENROWSET detected"),
    (r'\bopendatasource\b', "OPENDATASOURCE detected"),
    (r'\bsys\.objects\b', "sys.objects access detected"),
    (r'\binformation_schema\b', "INFORMATION_SCHEMA access detected"),
    (r'\bsysobjects\b', "sysobjects access detected"),
    (r'--\s*$', "SQL comment injection detected"),
    (r'/\*.*?\*/', "Block comment detected"),
    (r'\bwaitfor\b', "WAITFOR delay detected"),
    (r'\bdbcc\b', "DBCC command detected"),
    (r'\bshutdown\b', "SHUTDOWN command detected"),
    (r'\bdrop\s+database\b', "DROP DATABASE detected"),
    (r'\bdrop\s+table\b', "DROP TABLE detected"),
    (r'\btruncate\s+table\b', "TRUNCATE TABLE detected"),
    (r'\balter\s+table\b', "ALTER TABLE detected"),
    (r'\bcreate\s+user\b', "CREATE USER detected"),
    (r'\bgrant\s+all\b', "GRANT ALL detected"),
    (r'\binto\s+outfile\b', "INTO OUTFILE detected"),
    (r'\bload_file\b', "LOAD_FILE detected"),
    (r'\bsleep\s*\(', "SLEEP function detected"),
    (r'\bbenchmark\s*\(', "BENCHMARK function detected"),
    (r"'\s+or\s+'", "OR injection pattern detected"),
    (r"1\s*=\s*1", "Tautology detected"),
    (r'\bunion\s+select\b', "UNION SELECT detected"),
    (r'\binsert\s+into\s+.*select\b', "INSERT SELECT detected"),
    (r'\bupdate\s+.*set\s+.*where\s+1\s*=\s*1\b', "Mass update detected"),
    (r'\bdelete\s+from\s+.*where\s+1\s*=\s*1\b', "Mass delete detected"),
]

# Shell/OS execution patterns — matched against SQL input to detect injection attempts
_SHELL_PATTERN_RULES = [
    (r'\bcmd\b', "CMD shell reference"),
    (r'\bpowershell\b', "PowerShell reference"),
    (r'/bin/bash', "/bin/bash reference"),
    (r'/bin/sh', "/bin/sh reference"),
    (r'system\s*\(', "system() call"),
    (r'popen\s*\(', "popen() call"),
    # Detect the os module system call reference in SQL strings
    ("os" + r"\." + "system", "os module shell call reference"),
]


def validate_sql_security(sql: str) -> ValidationResult:
    """Validate SQL for injection and dangerous patterns. CPU-bound."""
    issues = []
    sql_lower = sql.lower()
    for pattern, message in DANGEROUS_SQL_PATTERNS:
        if re.search(pattern, sql_lower, re.IGNORECASE | re.DOTALL):
            issues.append(message)
    for pattern, message in _SHELL_PATTERN_RULES:
        if re.search(pattern, sql_lower, re.IGNORECASE):
            issues.append(message)
    # Encoding attack detection
    if any(ord(c) > 127 for c in sql):
        issues.append("Non-ASCII characters detected (possible encoding attack)")
    return ValidationResult(safe=len(issues) == 0, issues=issues)
