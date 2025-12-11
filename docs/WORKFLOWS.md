# Nest Project CI/CD Workflows

This document describes all GitHub Actions workflows used in the Nest project, including automation strategies, triggers, and compliance with `.WORKFLOW` standards.

## Workflow Overview

The Nest project implements a comprehensive CI/CD pipeline supporting multi-language development (Go, Python, Node.js) with automated testing, security scanning, and release management.

### Workflow Files

| Workflow | File | Trigger | Purpose |
|----------|------|---------|---------|
| Continuous Integration | `.github/workflows/ci.yml` | Push to main/develop/feature/*, Pull requests | Multi-language testing and security scanning |
| Version Monitoring | `.github/workflows/version-monitor.yml` | .version file changes | Version validation and consistency checks |
| Version Release | `.github/workflows/version-release.yml` | .version file push to main | Automated release creation |
| Push to Registry | `.github/workflows/push.yml` | Push to main | Docker image building and publishing |
| Release | `.github/workflows/release.yml` | GitHub Release published | Release-triggered operations |
| Docker Build | `.github/workflows/docker-build.yml` | Manual trigger | On-demand Docker builds |
| Deploy | `.github/workflows/deploy.yml` | Manual trigger | Deployment automation |
| Cron Scheduled | `.github/workflows/cron.yml` | Daily at 2 AM UTC | Scheduled maintenance tasks |
| GitStream | `.github/workflows/gitstream.yml` | Code analysis | Automated code review and suggestions |

## Version Management System

### .version File Format

The `.version` file uses the format: `vMajor.Minor.Patch.build`

**Example**: `1.0.0.1737727200`
- `1` = Major version (breaking changes)
- `0` = Minor version (new features)
- `0` = Patch version (bug fixes)
- `1737727200` = Epoch64 timestamp (build identifier)

### Version File Monitoring (version-monitor.yml)

**Triggers:**
- When `.version` file changes on main/develop branches
- During pull requests affecting `.version`

**Jobs:**
1. **validate-version**: Validates version format and consistency
   - Checks file exists
   - Validates semantic versioning format
   - Compares with previous commit
   - Logs version metadata

2. **version-consistency**: Ensures VERSION.md alignment
   - Verifies version references across documentation
   - Checks for consistency in source files

3. **build-validation**: Tests builds with current version
   - Go builds with `-ldflags` version injection
   - Python package builds
   - Node.js builds with version context

4. **security-check**: Security scanning with version context
   - gosec (Go security)
   - bandit (Python security)
   - npm audit (Node.js security)

### Automated Release Process (version-release.yml)

**Triggers:**
- Push to main branch when `.version` file is modified

**Functionality:**
- Detects version file changes
- Validates version is not default (0.0.0)
- Checks if release already exists
- Generates release notes with commit history
- Creates pre-release on GitHub
- Skips if version unchanged or release exists

## Continuous Integration Workflow

### CI Workflow (ci.yml)

**Triggers:**
- Push to main, develop, or feature/* branches
- Pull requests to main/develop
- Daily schedule at 2 AM UTC

**Environment Variables:**
```yaml
GO_VERSION: '1.23.5'
PYTHON_VERSION: '3.12'
NODE_VERSION: '18'
REGISTRY: ghcr.io
```

### Changes Detection

Uses dorny/paths-filter to skip unnecessary jobs:
- Detects changes in Go files (go.mod, go.sum, *.go, apps/api/**, services/**)
- Detects changes in Python files (requirements.txt, *.py, apps/web/**)
- Detects changes in Node.js (package.json, *.js, *.ts, *.tsx, web/src/**)
- Detects web and documentation changes

### Go Testing Job (go-test)

**Matrix:** Go 1.23.5, 1.24.0

**Steps:**
1. Checkout code
2. Set up Go with caching
3. Download and verify dependencies
4. Run go vet (static analysis)
5. Run staticcheck
6. Execute tests with race detector and coverage
7. Generate JUnit test reports
8. Upload coverage to Codecov

**Outputs:**
- coverage.out: Code coverage report
- test-results.xml: JUnit format for reporting

### Python Testing Job (python-test)

**Matrix:** Python 3.12, 3.13

**Services:**
- PostgreSQL 15 (test database)
- Redis 7 (cache testing)

**Steps:**
1. Checkout code
2. Set up Python with pip caching
3. Install dependencies (pytest, coverage, linting tools)
4. Black formatting check
5. isort import sorting check
6. flake8 linting (E9, F63, F7, F82)
7. mypy type checking (continue on error)
8. pytest with coverage (XML + HTML reports)
9. Upload coverage to Codecov

**Environment:**
```
DATABASE_URL: postgresql://test_user:test_pass@localhost:5432/test_db
REDIS_URL: redis://localhost:6379/1
LICENSE_KEY: PENG-TEST-TEST-TEST-TEST-ABCD
PRODUCT_NAME: test-product
```

### Node.js Testing Job (node-test)

**Matrix:** Node.js 18, 20, 22

**Steps:**
1. Checkout code
2. Set up Node.js with npm caching
3. Install root and web dependencies (npm ci)
4. ESLint linting
5. Prettier format checking
6. TypeScript type checking
7. Unit tests with Jest
8. Build web application
9. Upload build artifacts

**Artifacts:**
- web-build-{version}: Built web distribution

### Integration Testing Job (integration-test)

**Runs after:** All language-specific tests

**Services:**
- PostgreSQL 15
- Redis 7

**Steps:**
1. Set up all three environments (Go, Python, Node.js)
2. Download all dependencies
3. Build applications
4. Start services in background
5. Health checks on /health and /metrics endpoints
6. API endpoint validation
7. License info endpoint verification
8. E2E tests if available (Playwright)

### Security Scanning Job (security)

**Tools:**
- **gosec**: Go security scanning (SARIF format)
- **bandit**: Python vulnerability scanner
- **npm audit**: Node.js dependency auditing
- **Trivy**: Filesystem vulnerability scanning
- **CodeQL**: Language analysis (go, python, javascript)
- **Semgrep**: Pattern-based security analysis

**Outputs:**
- gosec-results.sarif: Go security findings
- trivy-results.sarif: Vulnerability scan results
- CodeQL alerts in GitHub Security tab

### License Validation Job (license-check)

**Validation:**
- Verifies license client integration across all languages
- Checks shared/licensing/client.go exists
- Checks shared/licensing/python_client.py exists
- Checks web/src/lib/license-client.js exists
- Confirms PENG- license key format in codebase

### Test Summary Job (test-summary)

**Runs after:** All tests complete (always)

**Steps:**
1. Downloads all test artifacts
2. Generates markdown summary
3. Comments on pull requests with results
4. Fails if any required test failed

## Security Scanning Standards

### Go Security (gosec)

**Configuration:**
```bash
args: '-no-fail -fmt sarif -out gosec-results.sarif ./...'
```

**Coverage:**
- Hardcoded credentials detection
- SQL injection analysis
- Cross-site scripting (XSS) detection
- Weak cryptography warnings
- Command injection detection

**Repository:** github.com/securego/gosec/v2

### Python Security (bandit)

**Installation:**
```bash
pip install bandit[toml]
```

**Configuration:**
- Recursive directory scan
- JSON output format
- Covers common Python vulnerabilities:
  - Hardcoded SQL queries
  - Insecure pickle usage
  - Temporary file creation
  - assert statement usage
  - Weak cryptography

### Node.js Security (npm audit)

**Levels:**
- low: Minor security issues
- moderate: Potential vulnerabilities
- high: Significant security risk
- critical: Immediate action required

**Command:**
```bash
npm audit --audit-level=moderate
```

## Docker Build Workflow

### Push Workflow (push.yml)

**Triggers:** Push to main branch

**Steps:**
1. Checkout code
2. Ansible lint for infrastructure code
3. CodeCov upload
4. Docker Hub login
5. GHCR login
6. Metadata extraction
7. Build and push to registries

**Registry Targets:**
- Docker Hub: penguincloud/{repo}
- GHCR: ghcr.io/{github-repository}

### Docker Build Workflow (docker-build.yml)

**Triggers:** Manual workflow dispatch

**Purpose:** On-demand Docker image building without automatic push

## Release Management

### Release Workflow (release.yml)

**Triggers:** When GitHub Release is published

**Functionality:**
- Extracts release metadata
- Builds release-specific artifacts
- Publishes to multiple registries
- Generates release notes

### Version Release Workflow (version-release.yml)

**Triggers:** Push to main when `.version` changes

**Process:**
1. Read .version file
2. Check if release already exists
3. Generate comprehensive release notes
4. Create pre-release with:
   - Semantic version
   - Full version with build timestamp
   - Commit SHA
   - Branch name
   - Automatic changelog

## Scheduled Tasks

### Cron Workflow (cron.yml)

**Schedule:** Daily at 2 AM UTC

**Tasks:**
- Dependency vulnerability checks
- Database schema consistency verification
- License server connectivity validation
- Container image updates

## GitStream Integration

### GitStream Workflow (gitstream.yml)

**Purpose:** Automated code review and analysis

**Features:**
- Detects security patterns
- Suggests documentation updates
- Identifies complexity issues
- Automates label assignment

## Compliance with .WORKFLOW Standards

### Version Monitoring Compliance

- ✅ `.version` file is monitored on every push
- ✅ Epoch64 timestamps for build identification
- ✅ Semantic versioning validation
- ✅ Version consistency checks across files
- ✅ Build validation with current version
- ✅ Security scanning with version context

### Security Scanning Compliance

- ✅ gosec for Go source code
- ✅ bandit for Python source code
- ✅ npm audit for Node.js dependencies
- ✅ Trivy for filesystem vulnerabilities
- ✅ CodeQL for advanced analysis
- ✅ Semgrep for pattern-based detection

### Multi-Language Support

- ✅ Go 1.23.5+ with race detector and coverage
- ✅ Python 3.12/3.13 with comprehensive linting
- ✅ Node.js 18/20/22 with modern tooling
- ✅ Database testing (PostgreSQL, Redis)
- ✅ Integration testing across all components

## Local Development Workflow

### Pre-commit Checks

Before pushing code, run locally:

**Go:**
```bash
go mod download
go mod verify
go vet ./...
golangci-lint run
go test -v -race -coverprofile=coverage.out ./...
```

**Python:**
```bash
pip install -r requirements.txt
pip install pytest pytest-cov pytest-asyncio black isort flake8 mypy bandit
black --check .
isort --check-only .
flake8 .
mypy .
pytest
```

**Node.js:**
```bash
npm install
npm run lint
npm run format -- --check
npm run typecheck
npm test
npm run build
```

## Environment Setup

### Required Secrets

Set in repository settings:

- `DOCKER_USERNAME`: Docker Hub username
- `DOCKER_PASSWORD`: Docker Hub token
- `GITHUB_TOKEN`: Automatically provided by GitHub Actions

### Codecov Integration

Coverage reports automatically uploaded to Codecov for:
- Go tests
- Python tests
- (Node.js coverage optional)

## Performance Optimization

### Caching Strategy

- Go modules cached in `~/.cache/go-build` and `~/go/pkg/mod`
- pip dependencies cached in `~/.cache/pip`
- npm packages cached via actions/setup-node

### Parallelization

- Go/Python/Node.js tests run in parallel
- Integration tests run after unit tests
- Security scanning runs independently

### Conditional Execution

- Jobs skip if no relevant file changes detected
- Tests skip if all changes are documentation
- Docker builds only on main branch

## Troubleshooting

### Version Validation Failures

If `.version` validation fails:
1. Check format: `vMajor.Minor.Patch` or `vMajor.Minor.Patch.build`
2. Ensure no whitespace in version string
3. Verify semantic versioning rules (incrementing)

### Security Scan False Positives

To suppress false positives:
- **Go**: Add `#nosec` comments with explanation
- **Python**: Update bandit configuration
- **Node.js**: Audit suppress in package.json

### Test Failures in CI

If tests pass locally but fail in CI:
1. Check environment variable differences
2. Verify database/cache service availability
3. Check for race conditions (use `-race` flag)
4. Review artifact dependencies

## Further Reading

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [GoLangCI-Lint](https://golangci-lint.run/)
- [Bandit Documentation](https://bandit.readthedocs.io/)
- [npm audit](https://docs.npmjs.com/cli/v8/commands/npm-audit)
- [Trivy Vulnerability Scanner](https://github.com/aquasecurity/trivy)
