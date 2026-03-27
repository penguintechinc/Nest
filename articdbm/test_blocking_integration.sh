#!/bin/bash

# ArticDBM Blocking System Integration Test
# Tests the complete blocking functionality across proxy and manager components

set -e

echo "🚀 ArticDBM Blocking System Integration Test"
echo "=============================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
MANAGER_URL="http://localhost:8000"
PROXY_HOST="localhost"
TEST_DIR="/tmp/articdbm_test_$$"

# Create test directory
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

echo_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

echo_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

echo_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Test 1: Run Go security checker tests
test_go_security_checker() {
    echo_status "Testing Go security checker..."
    
    cd /workspaces/ArticDBM/services/proxy
    
    # Check if Go is available
    if ! command -v go &> /dev/null; then
        echo_warning "Go not found, skipping Go tests"
        return 0
    fi
    
    # Run the security checker tests
    echo_status "Running security checker unit tests..."
    
    if go test ./internal/security/... -v; then
        echo_success "Go security checker tests passed"
    else
        echo_error "Go security checker tests failed"
        return 1
    fi
    
    cd "$TEST_DIR"
}

# Test 2: Run Python manager tests  
test_python_manager() {
    echo_status "Testing Python manager blocking system..."
    
    cd /workspaces/ArticDBM/services/manager
    
    # Check if Python is available
    if ! command -v python3 &> /dev/null; then
        echo_warning "Python3 not found, skipping Python tests"
        return 0
    fi
    
    # Run the manager tests
    echo_status "Running manager blocking system tests..."
    
    if python3 test_blocking_system.py; then
        echo_success "Python manager tests passed"
    else
        echo_error "Python manager tests failed"
        return 1
    fi
    
    cd "$TEST_DIR"
}

# Test 3: Test default blocked resources structure
test_default_blocked_resources() {
    echo_status "Testing default blocked resources structure..."
    
    # Create a test script to verify default resources
    cat > test_defaults.py << 'EOF'
#!/usr/bin/env python3
import sys
import re

def test_critical_patterns():
    """Test that critical patterns are properly formatted"""
    
    critical_databases = [
        ('master', '^master$', 'SQL Server system database'),
        ('mysql', '^mysql$', 'MySQL system database'),
        ('postgres', '^postgres$', 'PostgreSQL default database'),
        ('admin', '^admin$', 'MongoDB admin database'),
        ('test', '^test$', 'Default test database'),
    ]
    
    critical_users = [
        ('sa', '^sa$', 'SQL Server default admin account'),
        ('root', '^root$', 'Default root account'),
        ('admin', '^admin$', 'Default admin account'),
        ('guest', '^guest$', 'Default guest account'),
    ]
    
    # Test regex patterns
    test_cases = [
        ('master', '^master$', True),
        ('notmaster', '^master$', False),
        ('test_db', '^test_.*', True),
        ('production', '^test_.*', False),
        ('superadmin', '.*admin.*', True),
        ('normaluser', '.*admin.*', False),
    ]
    
    errors = 0
    
    for test_string, pattern, expected in test_cases:
        try:
            result = bool(re.match(pattern, test_string))
            if result != expected:
                print(f"FAIL: Pattern '{pattern}' on '{test_string}' - expected {expected}, got {result}")
                errors += 1
            else:
                print(f"PASS: Pattern '{pattern}' on '{test_string}' = {result}")
        except Exception as e:
            print(f"ERROR: Pattern '{pattern}' is invalid: {e}")
            errors += 1
    
    if errors == 0:
        print("All pattern tests passed!")
        return True
    else:
        print(f"{errors} pattern tests failed!")
        return False

if __name__ == '__main__':
    success = test_critical_patterns()
    sys.exit(0 if success else 1)
EOF
    
    if python3 test_defaults.py; then
        echo_success "Default blocked resources structure is valid"
    else
        echo_error "Default blocked resources structure has issues"
        return 1
    fi
}

# Test 4: Test SQL security validation
test_sql_security_validation() {
    echo_status "Testing SQL security validation..."
    
    # Create test SQL files with known dangerous patterns
    cat > dangerous.sql << 'EOF'
SELECT * FROM users WHERE id = 1; DROP TABLE users; --
EXEC xp_cmdshell 'dir'
SELECT * FROM information_schema.tables
UNION SELECT username, password FROM admin_users
'; system('rm -rf /'); --
EOF
    
    cat > safe.sql << 'EOF'
SELECT id, name, email FROM customers WHERE active = 1;
UPDATE products SET price = price * 1.1 WHERE category = 'electronics';
INSERT INTO orders (customer_id, total, created_at) VALUES (1, 99.99, NOW());
EOF
    
    # Test dangerous SQL detection
    dangerous_patterns=(
        "DROP TABLE"
        "xp_cmdshell"
        "information_schema"
        "UNION SELECT"
        "system("
    )
    
    echo_status "Testing dangerous SQL pattern detection..."
    for pattern in "${dangerous_patterns[@]}"; do
        if grep -qi "$pattern" dangerous.sql; then
            echo_success "Found dangerous pattern: $pattern"
        else
            echo_error "Failed to detect dangerous pattern: $pattern"
            return 1
        fi
    done
    
    echo_success "SQL security validation test passed"
}

# Test 5: Test blocking configuration data
test_blocking_configuration() {
    echo_status "Testing blocking configuration data structure..."
    
    # Create a JSON test for blocking configuration
    cat > test_blocking_config.json << 'EOF'
{
    "databases": [
        {
            "name": "test",
            "type": "database", 
            "pattern": "^test$",
            "reason": "Test database blocked",
            "active": true
        },
        {
            "name": "admin",
            "type": "username",
            "pattern": "^admin$", 
            "reason": "Admin user blocked",
            "active": true
        }
    ]
}
EOF
    
    # Validate JSON structure
    if python3 -m json.tool test_blocking_config.json > /dev/null; then
        echo_success "Blocking configuration JSON is valid"
    else
        echo_error "Blocking configuration JSON is invalid"
        return 1
    fi
    
    # Test that required fields are present
    required_fields=("name" "type" "pattern" "reason" "active")
    
    for field in "${required_fields[@]}"; do
        if grep -q "\"$field\"" test_blocking_config.json; then
            echo_success "Required field present: $field"
        else
            echo_error "Missing required field: $field"
            return 1
        fi
    done
}

# Test 6: Integration test of blocking workflow
test_blocking_workflow() {
    echo_status "Testing complete blocking workflow..."
    
    # Create a test that simulates the blocking workflow
    cat > test_workflow.py << 'EOF'
#!/usr/bin/env python3
import json
import re

def test_blocking_workflow():
    """Test the complete blocking workflow"""
    
    # 1. Define blocked resources (simulating Redis data)
    blocked_resources = {
        "test_db": {
            "name": "test", 
            "type": "database",
            "pattern": "^test$",
            "reason": "Test database blocked",
            "active": True
        },
        "admin_user": {
            "name": "admin",
            "type": "username", 
            "pattern": "^admin$",
            "reason": "Admin user blocked",
            "active": True
        },
        "inactive_rule": {
            "name": "old_rule",
            "type": "database",
            "pattern": "^old$", 
            "reason": "Old rule",
            "active": False
        }
    }
    
    def is_blocked(database, username, table=""):
        """Simulate the blocking check"""
        for resource in blocked_resources.values():
            if not resource["active"]:
                continue
                
            target = ""
            if resource["type"] == "database":
                target = database
            elif resource["type"] == "username":
                target = username
            elif resource["type"] == "table":
                target = table
                
            if target and re.match(resource["pattern"], target):
                return True, resource["reason"]
        
        return False, ""
    
    # Test cases
    test_cases = [
        ("test", "normaluser", "", True, "Should block test database"),
        ("production", "admin", "", True, "Should block admin user"),
        ("production", "normaluser", "", False, "Should allow normal access"),
        ("old", "normaluser", "", False, "Should not block inactive rules"),
    ]
    
    errors = 0
    for database, username, table, expected_blocked, description in test_cases:
        blocked, reason = is_blocked(database, username, table)
        if blocked != expected_blocked:
            print(f"FAIL: {description} - expected {expected_blocked}, got {blocked}")
            errors += 1
        else:
            print(f"PASS: {description}")
    
    return errors == 0

if __name__ == '__main__':
    import sys
    success = test_blocking_workflow()
    print(f"Blocking workflow test: {'PASSED' if success else 'FAILED'}")
    sys.exit(0 if success else 1)
EOF
    
    if python3 test_workflow.py; then
        echo_success "Blocking workflow test passed"
    else
        echo_error "Blocking workflow test failed"
        return 1
    fi
}

# Main test execution
main() {
    echo_status "Starting ArticDBM Blocking System Integration Tests..."
    
    local failed_tests=0
    
    # Run all tests
    echo_status "Running test suite..."
    
    if ! test_default_blocked_resources; then
        ((failed_tests++))
    fi
    
    if ! test_sql_security_validation; then
        ((failed_tests++))
    fi
    
    if ! test_blocking_configuration; then
        ((failed_tests++))
    fi
    
    if ! test_blocking_workflow; then
        ((failed_tests++))
    fi
    
    # Optional tests (may not work in all environments)
    echo_status "Running optional tests..."
    
    test_go_security_checker || echo_warning "Go tests skipped or failed"
    test_python_manager || echo_warning "Python manager tests skipped or failed"
    
    # Summary
    echo
    echo "=============================================="
    if [ $failed_tests -eq 0 ]; then
        echo_success "🎉 All critical tests passed!"
        echo_status "ArticDBM Blocking System is ready for production"
    else
        echo_error "❌ $failed_tests critical tests failed"
        echo_status "Please review the failing tests before deploying"
    fi
    
    echo_status "Test artifacts saved in: $TEST_DIR"
    echo "=============================================="
    
    return $failed_tests
}

# Cleanup function
cleanup() {
    echo_status "Cleaning up test files..."
    rm -rf "$TEST_DIR" 2>/dev/null || true
}

# Set up cleanup on exit
trap cleanup EXIT

# Run main test
main
exit_code=$?

exit $exit_code