# 🤝 Contributing to ArticDBM

Welcome to ArticDBM! We're excited to have you contribute to our open-source database proxy and management system. This guide will help you get started with contributing to the project.

## Table of Contents

- [🤝 Contributing to ArticDBM](#-contributing-to-articdbm)
  - [Table of Contents](#table-of-contents)
  - [📜 Code of Conduct](#-code-of-conduct)
  - [🏗️ Getting Started](#️-getting-started)
  - [🔧 Development Setup](#-development-setup)
  - [📝 Making Contributions](#-making-contributions)
  - [🧪 Testing](#-testing)
  - [📋 Code Style & Standards](#-code-style--standards)
  - [🔄 Pull Request Process](#-pull-request-process)
  - [🐛 Issue Reporting](#-issue-reporting)
  - [📖 Documentation](#-documentation)
  - [🎯 Areas for Contribution](#-areas-for-contribution)
  - [🏷️ Release Process](#️-release-process)
  - [👥 Community](#-community)
  - [📄 License](#-license)

## 📜 Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct:

- **Be respectful**: Treat everyone with respect and kindness
- **Be inclusive**: Welcome newcomers and help them get started
- **Be collaborative**: Work together and share knowledge
- **Be constructive**: Provide helpful feedback and suggestions
- **Be professional**: Maintain professional standards in all communications

## 🏗️ Getting Started

### Prerequisites

Before contributing, make sure you have:

- **Git** installed and configured
- **Docker** and **Docker Compose** for local development
- **Go 1.21+** for proxy development
- **Python 3.11+** for manager development
- **Node.js 18+** for frontend development (if applicable)

### First-Time Setup

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/your-username/articdbm.git
   cd articdbm
   ```

3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/penguintech-group/articdbm.git
   ```

4. **Start the development environment**:
   ```bash
   docker-compose up -d
   ```

## 🔧 Development Setup

### Local Development Environment

```bash
# Clone the repository
git clone https://github.com/your-username/articdbm.git
cd articdbm

# Copy environment template
cp .env.example .env

# Start development services
docker-compose -f docker-compose.dev.yml up -d

# Install development dependencies
make dev-setup
```

### Development Services

The development environment includes:

| Service | Port | Purpose |
|---------|------|---------|
| Manager | 8000 | Management API and web interface |
| Proxy | 3306, 5432, 1433, 27017, 6380 | Database proxies |
| Redis | 6379 | Configuration cache |
| PostgreSQL | 5433 | Metadata storage |
| Test DBs | Various | Test database backends |

### IDE Setup

#### Visual Studio Code

Install recommended extensions:
```json
{
  "recommendations": [
    "golang.go",
    "ms-python.python",
    "ms-python.black-formatter",
    "ms-python.flake8",
    "bradlc.vscode-tailwindcss",
    "ms-vscode.vscode-json"
  ]
}
```

#### GoLand/PyCharm

1. Import the project
2. Configure Go SDK (1.21+)
3. Configure Python interpreter (3.11+)
4. Set up run configurations

## 📝 Making Contributions

### Types of Contributions

We welcome various types of contributions:

- 🐛 **Bug fixes**
- ✨ **New features**
- 📝 **Documentation improvements**
- 🧪 **Test coverage improvements**
- 🔧 **Performance optimizations**
- 🛡️ **Security enhancements**
- 🌐 **Translations**

### Contribution Workflow

1. **Check existing issues** - Look for related issues or discussions
2. **Create an issue** - If none exists, create one to discuss your contribution
3. **Fork and branch** - Create a feature branch from `main`
4. **Develop** - Make your changes following our guidelines
5. **Test** - Ensure all tests pass and add new tests
6. **Commit** - Use conventional commit messages
7. **Push** - Push your changes to your fork
8. **Create PR** - Open a pull request with a clear description

### Branch Naming Convention

Use descriptive branch names:

```bash
# Features
git checkout -b feature/add-mongodb-support
git checkout -b feature/user-role-management

# Bug fixes
git checkout -b fix/connection-pool-leak
git checkout -b fix/sql-injection-detection

# Documentation
git checkout -b docs/api-reference-update
git checkout -b docs/deployment-guide

# Maintenance
git checkout -b chore/update-dependencies
git checkout -b refactor/proxy-architecture
```

### Commit Message Guidelines

Follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```bash
# Format
type(scope): description

# Examples
feat(proxy): add MongoDB protocol support
fix(manager): resolve connection pool memory leak
docs(api): update authentication endpoints
test(security): add SQL injection detection tests
refactor(proxy): improve connection handling
chore(deps): update Go dependencies to latest
```

Types:
- `feat`: New features
- `fix`: Bug fixes
- `docs`: Documentation changes
- `test`: Test additions/modifications
- `refactor`: Code refactoring
- `chore`: Maintenance tasks
- `ci`: CI/CD changes
- `perf`: Performance improvements

## 🧪 Testing

### Running Tests

```bash
# Run all tests
make test

# Run tests by component
make test-proxy
make test-manager
make test-integration

# Run with coverage
make test-coverage

# Run specific test files
go test ./services/proxy/internal/handlers/...
python -m pytest services/manager/tests/
```

### Test Categories

#### Unit Tests
- Test individual functions and components
- Mock external dependencies
- Fast execution (< 1s per test)

```go
// Example unit test
func TestSQLInjectionDetection(t *testing.T) {
    checker := security.NewSQLChecker(true)
    
    testCases := []struct {
        query    string
        expected bool
    }{
        {"SELECT * FROM users", false},
        {"SELECT * FROM users WHERE id = 1 OR 1=1", true},
        {"SELECT * FROM users UNION SELECT * FROM passwords", true},
    }
    
    for _, tc := range testCases {
        result := checker.IsSQLInjection(tc.query)
        assert.Equal(t, tc.expected, result)
    }
}
```

#### Integration Tests
- Test component interactions
- Use test databases
- Moderate execution time (< 30s per test)

```python
# Example integration test
def test_user_permission_flow():
    # Create user
    user = create_test_user("test@example.com")
    
    # Grant database permission
    grant_permission(user.id, "test_db", "users", ["read"])
    
    # Test permission check
    assert check_permission(user.email, "test_db", "users", "read")
    assert not check_permission(user.email, "test_db", "users", "write")
```

#### End-to-End Tests
- Test complete user workflows
- Use real database connections
- Longer execution time (< 5min per test)

### Writing Good Tests

1. **Test naming**: Use descriptive names that explain the scenario
2. **Test organization**: Group related tests in the same file
3. **Test data**: Use fixtures for consistent test data
4. **Assertions**: Use specific assertions with clear messages
5. **Cleanup**: Ensure tests clean up after themselves

```go
func TestProxyHandlesMultipleConnections(t *testing.T) {
    // Arrange
    proxy := setupTestProxy(t)
    defer proxy.Close()
    
    // Act
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            conn := connectToProxy(t, proxy.Address())
            defer conn.Close()
            
            _, err := conn.Query("SELECT 1")
            assert.NoError(t, err)
        }()
    }
    wg.Wait()
    
    // Assert
    assert.Equal(t, 0, proxy.ActiveConnections())
}
```

## 📋 Code Style & Standards

### Go Code Style

Follow standard Go conventions:

```go
// Package documentation
// Package handlers implements database protocol handlers for ArticDBM proxy.
package handlers

import (
    "context"
    "fmt"
    "net"
    
    "github.com/penguintechinc/articdbm/proxy/internal/config"
)

// Handler interface defines the contract for protocol handlers.
type Handler interface {
    // HandleConnection processes incoming database connections.
    HandleConnection(ctx context.Context, conn net.Conn) error
}

// MySQLHandler implements the Handler interface for MySQL protocol.
type MySQLHandler struct {
    config *config.Config
    logger *zap.Logger
}

// NewMySQLHandler creates a new MySQL protocol handler.
func NewMySQLHandler(cfg *config.Config, logger *zap.Logger) *MySQLHandler {
    return &MySQLHandler{
        config: cfg,
        logger: logger,
    }
}

// HandleConnection implements Handler interface.
func (h *MySQLHandler) HandleConnection(ctx context.Context, conn net.Conn) error {
    defer func() {
        if err := conn.Close(); err != nil {
            h.logger.Error("Failed to close connection", zap.Error(err))
        }
    }()
    
    // Implementation details...
    return nil
}
```

### Python Code Style

Follow PEP 8 and use type hints:

```python
"""User authentication and authorization module."""

from typing import Dict, List, Optional
from datetime import datetime

from pydantic import BaseModel, Field


class User(BaseModel):
    """User model for authentication."""
    
    id: int
    email: str = Field(..., description="User's email address")
    password_hash: str
    created_at: datetime
    is_active: bool = True


class AuthService:
    """Service for user authentication and authorization."""
    
    def __init__(self, db_connection: DatabaseConnection) -> None:
        """Initialize the authentication service.
        
        Args:
            db_connection: Database connection instance.
        """
        self._db = db_connection
    
    async def authenticate_user(
        self, 
        email: str, 
        password: str
    ) -> Optional[User]:
        """Authenticate user with email and password.
        
        Args:
            email: User's email address.
            password: User's password.
            
        Returns:
            User instance if authentication successful, None otherwise.
        """
        user = await self._get_user_by_email(email)
        if user and self._verify_password(password, user.password_hash):
            return user
        return None
    
    def _verify_password(self, password: str, password_hash: str) -> bool:
        """Verify password against hash."""
        # Implementation details...
        return True
```

### Code Formatting

Use automated formatting tools:

```bash
# Go formatting
gofmt -w .
golangci-lint run

# Python formatting
black .
flake8 .
mypy .

# Run all formatting
make format
make lint
```

### Documentation Standards

#### Code Documentation

```go
// Package-level documentation
// Package security provides SQL injection detection and query validation
// for the ArticDBM proxy system.
//
// The security package implements multiple layers of protection:
//   - Pattern-based SQL injection detection
//   - Heuristic analysis for suspicious queries
//   - Query validation and sanitization
//   - Audit logging for security events
package security

// SQLChecker provides SQL injection detection capabilities.
//
// The checker uses a combination of regex patterns and heuristic analysis
// to identify potentially malicious SQL queries.
type SQLChecker struct {
    enabled  bool
    patterns []*regexp.Regexp
}
```

#### API Documentation

```python
@action('api/servers', method=['POST'])
@action.uses(auth, cors, db)
def create_server():
    """Create a new database server configuration.
    
    Creates a new database server entry that can be used as a backend
    for the ArticDBM proxy. The server configuration is validated and
    stored in the metadata database, then synchronized to Redis.
    
    Request Body:
        name (str): Unique identifier for the server
        type (str): Database type (mysql, postgresql, etc.)
        host (str): Server hostname or IP address
        port (int): Server port number
        username (str, optional): Database username
        password (str, optional): Database password
        database (str, optional): Default database name
        role (str): Server role (read, write, both)
        weight (int): Load balancing weight (default: 1)
        tls_enabled (bool): Whether to use TLS connection
        
    Returns:
        dict: Response containing server ID and success message
        
    Raises:
        400: If server configuration is invalid
        409: If server name already exists
        500: If database operation fails
        
    Example:
        >>> POST /api/servers
        >>> {
        ...     "name": "production-mysql",
        ...     "type": "mysql", 
        ...     "host": "mysql.example.com",
        ...     "port": 3306,
        ...     "role": "both"
        ... }
        <<< {
        ...     "id": 123,
        ...     "message": "Server created successfully"
        ... }
    """
```

## 🔄 Pull Request Process

### Before Creating a PR

1. **Sync with upstream**:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run tests locally**:
   ```bash
   make test-all
   make lint
   ```

3. **Update documentation** if needed

### PR Title and Description

Use a clear, descriptive title:

```
feat(proxy): add MongoDB protocol support with connection pooling

- Implement MongoDB wire protocol handler
- Add connection pooling for MongoDB backends  
- Include comprehensive test coverage
- Update documentation for MongoDB configuration

Closes #123
```

### PR Template

```markdown
## Description
Brief description of changes and motivation.

## Type of Change
- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] This change requires a documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Manual testing completed

## Checklist
- [ ] My code follows the style guidelines of this project
- [ ] I have performed a self-review of my own code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [ ] I have made corresponding changes to the documentation
- [ ] My changes generate no new warnings
- [ ] I have added tests that prove my fix is effective or that my feature works
- [ ] New and existing unit tests pass locally with my changes

## Additional Notes
Any additional information, configuration changes, or notes for reviewers.
```

### Review Process

1. **Automated checks** must pass (CI/CD, tests, linting)
2. **Code review** by at least one maintainer
3. **Documentation review** if applicable
4. **Security review** for security-related changes
5. **Performance review** for performance-critical changes

### Addressing Review Feedback

```bash
# Make changes based on feedback
git add .
git commit -m "address review feedback: improve error handling"

# Push changes
git push origin feature/your-branch-name
```

## 🐛 Issue Reporting

### Before Creating an Issue

1. **Search existing issues** to avoid duplicates
2. **Check the documentation** for known solutions
3. **Try the latest version** to see if the issue is already fixed

### Issue Templates

#### Bug Report

```markdown
**Bug Description**
A clear and concise description of what the bug is.

**Steps to Reproduce**
1. Go to '...'
2. Click on '....'
3. Scroll down to '....'
4. See error

**Expected Behavior**
A clear and concise description of what you expected to happen.

**Actual Behavior**
A clear and concise description of what actually happened.

**Environment**
- ArticDBM Version: [e.g. 1.0.0]
- OS: [e.g. Ubuntu 20.04]
- Docker Version: [e.g. 20.10.12]
- Database Backend: [e.g. MySQL 8.0]

**Logs**
```
Relevant log output here
```

**Additional Context**
Add any other context about the problem here.
```

#### Feature Request

```markdown
**Is your feature request related to a problem? Please describe.**
A clear and concise description of what the problem is. Ex. I'm always frustrated when [...]

**Describe the solution you'd like**
A clear and concise description of what you want to happen.

**Describe alternatives you've considered**
A clear and concise description of any alternative solutions or features you've considered.

**Additional context**
Add any other context or screenshots about the feature request here.
```

## 📖 Documentation

### Types of Documentation

1. **API Documentation** - Generated from code comments
2. **User Guides** - Step-by-step instructions for users
3. **Architecture Documentation** - System design and technical details
4. **Deployment Guides** - Installation and configuration instructions

### Writing Documentation

- Use clear, concise language
- Include code examples where helpful
- Add diagrams for complex concepts
- Keep documentation up-to-date with code changes

### Documentation Structure

```
docs/
├── README.md          # Main documentation index
├── usage.md           # User guide and examples
├── architecture.md    # System architecture
├── api.md            # API reference
├── security.md       # Security features
├── deployment.md     # Deployment guides
├── contributing.md   # This file
└── release-notes.md  # Version changelog
```

## 🎯 Areas for Contribution

### High-Priority Areas

1. **🔒 Security Enhancements**
   - Advanced threat detection
   - Audit compliance features  
   - Encryption improvements

2. **⚡ Performance Optimizations**
   - Connection pooling improvements
   - Query caching
   - Memory usage optimization

3. **🌐 Protocol Support**
   - Additional database protocols
   - Protocol version updates
   - Feature completeness

4. **📊 Monitoring & Observability**
   - Additional metrics
   - Distributed tracing
   - Dashboard improvements

### Beginner-Friendly Issues

Look for issues labeled with:
- `good first issue`
- `help wanted`
- `documentation`
- `testing`

### Advanced Contributions

- Core architecture improvements
- New protocol implementations
- Performance critical optimizations
- Security enhancements

## 🏷️ Release Process

### Versioning

We follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Incompatible API changes
- **MINOR**: Backwards-compatible functionality additions
- **PATCH**: Backwards-compatible bug fixes

### Release Schedule

- **Major releases**: Every 6 months
- **Minor releases**: Monthly
- **Patch releases**: As needed for critical fixes

### Contributing to Releases

1. **Feature freeze** - 2 weeks before major/minor releases
2. **Testing period** - 1 week of intensive testing
3. **Release candidate** - RC builds for final validation
4. **Release** - Tagged release with changelog

## 👥 Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and discussions
- **Discord Server**: Real-time chat and support
- **Mailing List**: Development announcements

### Community Guidelines

- Be respectful and inclusive
- Help newcomers get started
- Share knowledge and best practices
- Provide constructive feedback
- Follow the code of conduct

### Getting Help

1. **Read the documentation** first
2. **Search existing issues** and discussions
3. **Ask questions** in GitHub Discussions
4. **Join our Discord** for real-time help

## 📄 License

### License Overview

ArticDBM is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)**. 

### Contributor License Agreement

By contributing to ArticDBM, you agree that:

1. Your contributions will be licensed under AGPL-3.0
2. You have the right to submit the contributions
3. You grant Penguin Technologies Group the right to use your contributions

### Commercial Licensing

For commercial licensing options that allow closed-source usage, please contact:
- **Email**: enterprise@penguintech.group
- **Website**: https://penguintech.group/licensing

---

## 🚀 Ready to Contribute?

1. **Fork the repository**
2. **Set up your development environment**
3. **Pick an issue** or propose a new feature
4. **Make your contribution**
5. **Submit a pull request**

Thank you for contributing to ArticDBM! Together, we're building the future of database management and security.

---

*For questions about contributing, please reach out to us at contributors@penguintech.group*
