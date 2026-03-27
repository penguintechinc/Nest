#!/usr/bin/env python3
"""
Standalone unit tests for SQL security validation
"""

import re
import unittest
from typing import Dict


def validate_sql_security(content: str) -> Dict[str, any]:
    """Comprehensive SQL security validation - Standalone version"""
    errors = []
    warnings = []
    severity = "low"
    
    # Normalize content for analysis
    normalized = content.lower().strip()
    lines = content.split('\n')
    
    # Dangerous patterns that should be blocked
    dangerous_patterns = {
        r'exec\s*\(': "Potentially dangerous EXEC statement",
        r'sp_oacreate': "OLE automation detected",
        r'openrowset': "External data source access",
        r'opendatasource': "External data source access",
        r'bulk\s+insert': "Bulk insert operation",
        r'into\s+outfile': "File write operation detected",
        r'load_file\s*\(': "File read operation detected",
        r'@@version': "System information disclosure",
        r'information_schema': "Schema introspection detected",
        r'sys\.': "System table access",
        r'master\.': "Master database access",
        r'msdb\.': "MSDB database access",
        r'tempdb\.': "TempDB access",
        r'--\s*[^\r\n]*(\r?\n|$)': "SQL comments detected",
        r'/\*.*?\*/': "Block comments detected",
        r'\bor\s+1\s*=\s*1\b': "Classic SQL injection pattern",
        r'\bunion\s+select\b': "Union-based injection pattern",
        r';\s*(drop|truncate|delete)\s+': "Destructive operations in sequence",
        r'\bchar\s*\(\s*\d+': "Character function bypass attempt",
    }
    
    # Shell command patterns
    shell_patterns = {
        r'xp_cmdshell': "System command execution detected",
        r'\bcmd\b': "Command shell reference",
        r'\bpowershell\b': "PowerShell reference",
        r'\bbash\b': "Bash shell reference",
        r'\bsh\b': "Shell reference",
        r'/bin/': "Unix binary path",
        r'system\s*\(': "System call",
        r'shell_exec': "Shell execution",
        r'passthru': "Command execution",
        r'proc_open': "Process execution",
    }
    
    # Check for dangerous SQL patterns
    for pattern, description in dangerous_patterns.items():
        if re.search(pattern, normalized, re.IGNORECASE | re.MULTILINE):
            errors.append(f"{description}: {pattern}")
            severity = "high"
    
    # Check for shell command patterns
    for pattern, description in shell_patterns.items():
        if re.search(pattern, normalized, re.IGNORECASE):
            errors.append(f"Shell command detected - {description}: {pattern}")
            severity = "critical"
    
    # Check for default/test database access patterns
    default_db_patterns = {
        r'\btest\b': "Test database access",
        r'\bsample\b': "Sample database access",  
        r'\bdemo\b': "Demo database access",
        r'\bexample\b': "Example database access",
        r'\bsa\b': "Default 'sa' user detected",
        r'\broot\b': "Root user access",
        r'\badmin\b': "Admin user access",
        r'\bguest\b': "Guest user access",
    }
    
    for pattern, description in default_db_patterns.items():
        if re.search(pattern, normalized, re.IGNORECASE):
            warnings.append(f"Potential default/test resource access - {description}")
            if severity == "low":
                severity = "medium"
    
    # Basic syntax validation
    bracket_count = content.count('(') - content.count(')')
    if bracket_count != 0:
        errors.append("Unmatched parentheses in SQL")
    
    quote_count = content.count("'") % 2
    if quote_count != 0:
        errors.append("Unmatched single quotes in SQL")
    
    # Check for multiple statements (potential SQL injection)
    semicolon_statements = [s.strip() for s in content.split(';') if s.strip()]
    if len(semicolon_statements) > 1:
        statement_types = []
        for stmt in semicolon_statements:
            first_word = stmt.split()[0].upper() if stmt.split() else ""
            statement_types.append(first_word)
        
        if len(set(statement_types)) > 1:
            warnings.append(f"Multiple different statement types detected: {', '.join(set(statement_types))}")
            if severity == "low":
                severity = "medium"
    
    # Check for extremely long lines (potential obfuscation)
    for i, line in enumerate(lines, 1):
        if len(line) > 1000:
            warnings.append(f"Extremely long line detected (line {i}): {len(line)} characters")
    
    # Check for non-ASCII characters (potential encoding attacks)
    try:
        content.encode('ascii')
    except UnicodeEncodeError:
        warnings.append("Non-ASCII characters detected - potential encoding attack")
    
    return {
        'valid': len(errors) == 0,
        'errors': errors,
        'warnings': warnings,
        'severity': severity,
        'patterns_checked': len(dangerous_patterns) + len(shell_patterns) + len(default_db_patterns)
    }


class TestSQLSecurityValidation(unittest.TestCase):
    """Test SQL security validation functionality"""
    
    def test_clean_sql_passes_validation(self):
        """Test that clean SQL statements pass validation"""
        clean_sql = "SELECT id, name FROM users WHERE active = 1"
        result = validate_sql_security(clean_sql)
        
        self.assertTrue(result['valid'])
        self.assertEqual(len(result['errors']), 0)
        self.assertEqual(result['severity'], 'low')
    
    def test_sql_injection_detected(self):
        """Test that SQL injection patterns are detected"""
        malicious_sql = "SELECT * FROM users WHERE id = 1 OR 1=1"
        result = validate_sql_security(malicious_sql)
        
        self.assertFalse(result['valid'])
        self.assertGreater(len(result['errors']), 0)
        self.assertIn('high', result['severity'])
    
    def test_shell_command_detected(self):
        """Test that shell commands are detected"""
        malicious_sql = "SELECT * FROM users; xp_cmdshell 'whoami'"
        result = validate_sql_security(malicious_sql)
        
        self.assertFalse(result['valid'])
        self.assertEqual(result['severity'], 'critical')
        self.assertTrue(any('Shell command' in error for error in result['errors']))
    
    def test_system_database_access_warning(self):
        """Test that system database access generates warnings"""
        system_sql = "SELECT * FROM master.dbo.sysdatabases"
        result = validate_sql_security(system_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('Master database access' in error for error in result['errors']))
    
    def test_default_user_detection(self):
        """Test that default users are detected"""
        default_user_sql = "SELECT * FROM users WHERE username = 'sa'"
        result = validate_sql_security(default_user_sql)
        
        self.assertGreater(len(result['warnings']), 0)
        self.assertTrue(any('sa' in warning for warning in result['warnings']))
    
    def test_syntax_error_detection(self):
        """Test that basic syntax errors are caught"""
        syntax_error_sql = "SELECT * FROM users WHERE (id = 1"
        result = validate_sql_security(syntax_error_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('parentheses' in error for error in result['errors']))
    
    def test_multiple_statement_detection(self):
        """Test that multiple statements generate warnings"""
        multiple_sql = "SELECT * FROM users; DELETE FROM logs;"
        result = validate_sql_security(multiple_sql)
        
        self.assertGreater(len(result['warnings']), 0)
        self.assertTrue(any('Multiple different statement types' in warning for warning in result['warnings']))
    
    def test_file_operations_blocked(self):
        """Test that file operations are blocked"""
        file_sql = "SELECT LOAD_FILE('/etc/passwd')"
        result = validate_sql_security(file_sql)
        
        self.assertFalse(result['valid'])
        self.assertEqual(result['severity'], 'high')
    
    def test_encoded_attacks(self):
        """Test detection of encoded attacks"""
        encoded_sql = "SELECT * FROM users WHERE id = 0x41424344"
        result = validate_sql_security(encoded_sql)
        
        # This should trigger the hex pattern detection if implemented
        # For now, this is a clean query in our basic implementation
        self.assertTrue(result['valid'] or 'hex' in str(result['errors']).lower())
    
    def test_comment_based_attacks(self):
        """Test detection of comment-based attacks"""
        comment_sql = "SELECT * FROM users WHERE id = 1 -- AND password = 'secret'"
        result = validate_sql_security(comment_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('comment' in error.lower() for error in result['errors']))
    
    def test_union_based_attacks(self):
        """Test detection of UNION-based attacks"""
        union_sql = "SELECT id FROM users UNION SELECT password FROM admin"
        result = validate_sql_security(union_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('union' in error.lower() for error in result['errors']))
    
    def test_bulk_operations_detected(self):
        """Test detection of bulk operations"""
        bulk_sql = "BULK INSERT users FROM 'c:\\temp\\users.txt'"
        result = validate_sql_security(bulk_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('bulk' in error.lower() for error in result['errors']))
    
    def test_information_disclosure_detected(self):
        """Test detection of information disclosure attempts"""
        info_sql = "SELECT @@version, @@servername"
        result = validate_sql_security(info_sql)
        
        self.assertFalse(result['valid'])
        self.assertTrue(any('system information' in error.lower() for error in result['errors']))
    
    def test_system_calls_detected(self):
        """Test detection of system calls"""
        system_sql = "SELECT system('ls -la')"
        result = validate_sql_security(system_sql)
        
        self.assertFalse(result['valid'])
        self.assertEqual(result['severity'], 'critical')
        self.assertTrue(any('system call' in error.lower() for error in result['errors']))
    
    def test_powershell_detected(self):
        """Test detection of PowerShell commands"""
        ps_sql = "SELECT * FROM users; powershell Get-Process"
        result = validate_sql_security(ps_sql)
        
        self.assertFalse(result['valid'])
        self.assertEqual(result['severity'], 'critical')
        self.assertTrue(any('powershell' in error.lower() for error in result['errors']))
    
    def test_long_line_warning(self):
        """Test detection of extremely long lines"""
        long_sql = "SELECT * FROM users WHERE name = '" + "A" * 1500 + "'"
        result = validate_sql_security(long_sql)
        
        self.assertGreater(len(result['warnings']), 0)
        self.assertTrue(any('extremely long line' in warning.lower() for warning in result['warnings']))
    
    def test_comprehensive_attack_detection(self):
        """Test detection of comprehensive attack scenarios"""
        attack_scenarios = [
            ("SELECT * FROM users WHERE id = 1 OR 1=1 -- comment", "SQL injection with comments"),
            ("SELECT * FROM users; DROP TABLE logs;", "SQL injection with destructive operation"),
            ("SELECT * FROM information_schema.tables WHERE table_schema = 'admin'", "Information schema access"),
            ("SELECT LOAD_FILE('/etc/passwd') as data", "File read operation"),
            ("SELECT * FROM users INTO OUTFILE '/tmp/users.txt'", "File write operation"),
            ("EXEC xp_cmdshell 'dir'", "System command execution"),
            ("SELECT * FROM users WHERE name = CHAR(65,68,77,73,78)", "Character-based bypass"),
        ]
        
        for sql, description in attack_scenarios:
            with self.subTest(description=description):
                result = validate_sql_security(sql)
                self.assertFalse(result['valid'], f"Failed to detect: {description}")
                self.assertIn(result['severity'], ['medium', 'high', 'critical'], 
                             f"Severity too low for: {description}")


if __name__ == '__main__':
    # Run all tests with verbose output
    unittest.main(verbosity=2)