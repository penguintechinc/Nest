# Nest Project Development & CI/CD Standards

This document outlines the standardized practices, code quality requirements, and CI/CD compliance standards for the Nest project.

## Table of Contents

1. [Version Management](#version-management)
2. [Code Quality Standards](#code-quality-standards)
3. [Security Standards](#security-standards)
4. [Testing Standards](#testing-standards)
5. [CI/CD Compliance](#cicd-compliance)
6. [Language-Specific Standards](#language-specific-standards)
7. [Documentation Standards](#documentation-standards)
8. [Release Process](#release-process)

## Version Management

### Version File Format

All releases use semantic versioning with build timestamp: `vMajor.Minor.Patch.build`

**Examples:**
- `1.0.0.1737727200` - Production release with Epoch64 timestamp
- `0.1.0` - Development release without timestamp
- `2.3.1.1737803600` - Patch release with build metadata

### Version Increment Rules

| Type | Change | Example |
|------|--------|---------|
| Major | Breaking changes, API changes, removed features | 1.x.x → 2.0.0 |
| Minor | New features, non-breaking enhancements | 1.0.x → 1.1.0 |
| Patch | Bug fixes, security patches | 1.0.0 → 1.0.1 |
| Build | Build metadata (timestamp only) | 1.0.0 → 1.0.0.1737727200 |

### Version File Location

- **Primary**: `.version` at project root
- **Documentation**: `VERSION.md` (optional, should match .version)
- **Source Code**: Injected via `-ldflags` during builds

## Code Quality Standards

### Universal Requirements

All code MUST:
- ✅ Pass linting without exceptions
- ✅ Include comprehensive error handling
- ✅ Contain appropriate logging at multiple levels
- ✅ Follow security-first design principles
- ✅ Have unit tests covering all code paths
- ✅ Pass CodeQL security analysis
- ✅ Avoid hardcoded credentials or secrets
- ✅ Use typed variables and functions

### Go Standards

**Minimum Version:** Go 1.23.5+

**Required Tools:**
- golangci-lint
- go fmt
- go vet
- staticcheck
- gosec

**Code Style:**
```go
// Use meaningful variable names
// Always handle errors explicitly
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}

// Use interfaces for flexibility
type Repository interface {
    Get(ctx context.Context, id string) (*Model, error)
}

// Prefer concrete error types
var (
    ErrNotFound = errors.New("resource not found")
    ErrInvalid  = errors.New("invalid input")
)
```

**Testing Requirements:**
- Unit tests with 80%+ coverage
- Race detector enabled (`-race` flag)
- Benchmark tests for performance-critical code
- Integration tests with real services (when applicable)

**Package Structure:**
```
cmd/
  appname/
    main.go
internal/
  pkg1/
    types.go
    impl.go
pkg/
  public/
    interface.go
```

### Python Standards

**Minimum Version:** Python 3.12+

**Code Style:** PEP 8, PEP 257, PEP 484

**Required Tools:**
- black (code formatting)
- isort (import sorting)
- flake8 (linting)
- mypy (type checking)
- bandit (security)
- pytest (testing)

**Code Example:**
```python
"""Module docstring describing purpose and exports."""

from typing import Optional, List
from dataclasses import dataclass
import logging

logger = logging.getLogger(__name__)

@dataclass
class User:
    """Represents a system user."""
    id: str
    name: str
    email: str

def get_user(user_id: str) -> Optional[User]:
    """Retrieve user by ID.

    Args:
        user_id: The unique user identifier

    Returns:
        User object if found, None otherwise

    Raises:
        ValueError: If user_id is empty
    """
    if not user_id:
        raise ValueError("user_id cannot be empty")

    logger.info(f"Fetching user: {user_id}")
    # Implementation
```

**Type Hints:** Mandatory for all functions and class attributes

**Testing:**
- pytest for unit tests
- pytest-cov for coverage
- pytest-asyncio for async tests
- Mock external dependencies
- No external network calls in unit tests

### Node.js/TypeScript Standards

**Minimum Version:** Node.js 18+

**Required Tools:**
- ESLint (linting)
- Prettier (formatting)
- TypeScript (type checking)
- Jest (testing)

**Code Style:**
```typescript
// Always use const/let, never var
const x = 1;

// Type all function parameters and returns
function processData(items: Item[]): Result[] {
    return items.map(item => transform(item));
}

// Use interfaces for type definitions
interface Config {
    timeout: number;
    retries: number;
}

// Prefer async/await
async function fetchData(url: string): Promise<Data> {
    const response = await fetch(url);
    return response.json();
}
```

**Testing:**
- Jest unit tests with 80%+ coverage
- No hardcoded test data
- Mock external services
- E2E tests for critical flows

## Security Standards

### Input Validation

**Rule:** ALL inputs from external sources MUST be validated

**Examples:**
```go
// Go validation
if len(id) == 0 || len(id) > 36 {
    return errors.New("invalid id format")
}

// Python validation
if not email or not re.match(r'^[\w\.-]+@[\w\.-]+\.\w+$', email):
    raise ValueError("invalid email format")
```

### Authentication & Authorization

- Implement role-based access control (RBAC)
- Never store plaintext passwords
- Use industry-standard hashing (bcrypt minimum)
- Implement token expiration
- Validate permissions on every protected endpoint

### Dependency Security

**Requirements:**
- Check for Dependabot alerts weekly
- Address all critical/high vulnerabilities immediately
- Keep dependencies current (within 3 months of latest)
- Use dependency scanning tools:
  - Go: `go mod audit`
  - Python: `safety check`
  - Node.js: `npm audit`

### Secrets Management

**Rules:**
- NEVER commit credentials to repository
- Use environment variables for secrets
- Rotate credentials regularly
- Audit secret access
- Use .gitignore for local secret files

**Files to exclude:**
```
.env
.env.local
secrets/
certs/private/
```

### HTTPS/TLS Requirements

- Enforce TLS 1.2 minimum (prefer TLS 1.3)
- Use valid certificates for all services
- Implement certificate rotation
- Regular security audits

## Testing Standards

### Unit Testing

**Requirements:**
- Test all exported functions
- Cover happy path and error cases
- Test boundary conditions
- Isolated from external dependencies
- Fast execution (milliseconds)

**Coverage Targets:**
- Minimum 80% code coverage
- 100% coverage for security-critical code
- 100% coverage for public APIs

**Unit Test Structure:**
```go
// Go example
func TestGetUser(t *testing.T) {
    tests := []struct {
        name    string
        userID  string
        want    *User
        wantErr bool
    }{
        {
            name:   "valid user",
            userID: "123",
            want:   &User{ID: "123", Name: "John"},
        },
        {
            name:    "empty id",
            userID:  "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Integration Testing

**Scope:**
- API endpoint functionality
- Database operations
- Message queue integration
- External service mocking

**Execution:**
- Run after unit tests
- Use real database instances
- Clean up test data
- Handle transient failures gracefully

### Performance Testing

**When Required:**
- Load-sensitive endpoints
- Database query optimization
- Cache performance

**Tools:**
- Go: testing.B benchmarks
- Python: pytest-benchmark
- Node.js: autocannon or custom

## CI/CD Compliance

### Build Requirements

✅ **Mandatory:**
- All code must compile/parse without errors
- All linting checks must pass
- All tests must pass
- Code coverage must meet thresholds
- Security scanning must complete

❌ **Prohibited:**
- Committed build artifacts
- Skipped failing tests
- Disabled security checks
- Hardcoded configuration

### Pull Request Requirements

Before merging to main:
1. ✅ All CI checks pass
2. ✅ Code review approval (minimum 1)
3. ✅ Security scan passes
4. ✅ Test coverage meets threshold
5. ✅ Documentation updated
6. ✅ Version number updated (if applicable)

### Deployment Requirements

**Development:**
- Can deploy from develop branch
- Requires passing CI
- No production data

**Production:**
- Only from main/release branches
- Requires successful release workflow
- Manual approval gate
- Change documentation required

## Language-Specific Standards

### Go-Specific

**gofmt:** All code automatically formatted
```bash
gofmt -s -w .
```

**go vet:** Static analysis for correctness
```bash
go vet ./...
```

**golangci-lint:** Combined linting (staticcheck, gosec, etc.)
```bash
golangci-lint run --timeout=5m
```

**Testing with Coverage:**
```bash
go test -v -race -coverprofile=coverage.out ./...
```

### Python-Specific

**black:** Deterministic formatting
```bash
black --line-length=100 .
```

**isort:** Import statement sorting
```bash
isort --profile black .
```

**flake8:** Style guide enforcement
```bash
flake8 . --max-line-length=100
```

**mypy:** Static type checking
```bash
mypy . --ignore-missing-imports
```

**bandit:** Security issue scanning
```bash
bandit -r . -f json
```

### Node.js/TypeScript-Specific

**ESLint:** Code quality and patterns
```bash
npm run lint
```

**Prettier:** Code formatting
```bash
npm run format -- --check
```

**TypeScript:** Type checking
```bash
npm run typecheck
```

**Jest:** Unit testing
```bash
npm test -- --coverage
```

## Documentation Standards

### Code Documentation

**Go:**
- Every exported function/type has a comment
- Comments are sentences starting with name
- Packages have a doc comment

```go
// User represents a system user with authentication details.
type User struct {
    ID    string
    Email string
}

// GetUser retrieves a user by ID from the database.
func GetUser(ctx context.Context, id string) (*User, error) {
```

**Python:**
- Docstrings for all modules, classes, functions
- Follow PEP 257 standard
- Include parameter and return types

```python
"""user module provides user management functionality."""

def get_user(user_id: str) -> Optional[User]:
    """Retrieve a user by ID.

    Args:
        user_id: Unique user identifier

    Returns:
        User object or None if not found

    Raises:
        ValueError: If user_id is invalid
    """
```

**TypeScript:**
- JSDoc comments for public APIs
- Type annotations mandatory
- Comments for complex logic

### Project Documentation

**Required Files:**
- README.md (overview and quick start)
- CONTRIBUTING.md (contribution guidelines)
- docs/WORKFLOWS.md (this file)
- docs/STANDARDS.md (standards documentation)
- CHANGELOG.md (version history)

**README Contents:**
- Build status badges
- Quick start guide
- Architecture overview
- Installation instructions
- License information

### API Documentation

- OpenAPI/Swagger specifications
- Request/response examples
- Authentication requirements
- Rate limiting details
- Error codes and meanings

## Release Process

### Version Bump Procedure

1. **Update .version file**
   ```bash
   # Check current version
   cat .version

   # For patch release
   ./scripts/version/update-version.sh patch

   # For minor release
   ./scripts/version/update-version.sh minor

   # For major release
   ./scripts/version/update-version.sh major
   ```

2. **Update documentation**
   - Update VERSION.md
   - Update CHANGELOG.md with changes
   - Update installation instructions if needed

3. **Create pull request**
   - Title: "Release v{version}"
   - Description: Summary of changes
   - Links to related issues

4. **Merge to main**
   - Require approval
   - All CI checks must pass

5. **Automatic release creation**
   - GitHub Actions creates release automatically
   - Release notes generated from CHANGELOG
   - Pre-release tag applied

### Release Candidate Process

For major releases, use release candidates:

1. **Branch:** Create `release/v1.0.0-rc1` from develop
2. **Testing:** Extended testing period (1-2 weeks)
3. **Bug fixes:** Apply only critical bug fixes
4. **Release:** When stable, merge to main with version bump
5. **Promotion:** Release → Release (GA) once stable in production

## Compliance Checklist

Before committing code:
- ✅ All files pass linting
- ✅ All tests pass locally
- ✅ Code coverage meets threshold
- ✅ Security scan passes locally
- ✅ No hardcoded secrets
- ✅ Error handling complete
- ✅ Logging appropriate
- ✅ Documentation updated
- ✅ Related issues linked
- ✅ Version file updated (if applicable)

Before merging pull request:
- ✅ CI pipeline fully passes
- ✅ Code review approved
- ✅ Security scan passes
- ✅ Test coverage meets threshold
- ✅ No merge conflicts
- ✅ Documentation complete
- ✅ Changelog updated

## Tools Reference

| Tool | Language | Purpose | Command |
|------|----------|---------|---------|
| golangci-lint | Go | Linting | `golangci-lint run` |
| gosec | Go | Security | `gosec ./...` |
| black | Python | Formatting | `black .` |
| bandit | Python | Security | `bandit -r .` |
| mypy | Python | Type checking | `mypy .` |
| ESLint | Node.js | Linting | `npm run lint` |
| Prettier | Node.js | Formatting | `npm run format` |
| Jest | All | Testing | `npm test` |

## References

- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [PEP 8 Style Guide](https://www.python.org/dev/peps/pep-0008/)
- [TypeScript Handbook](https://www.typescriptlang.org/docs/)
- [OWASP Secure Coding](https://owasp.org/www-project-secure-coding-practices-quick-reference-guide/)
- [Semantic Versioning](https://semver.org/)
