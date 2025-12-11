# Project Template - Claude Code Context

## Project Overview

This is a comprehensive project template incorporating best practices and patterns from Penguin Tech Inc projects. It provides a standardized foundation for multi-language projects with enterprise-grade infrastructure and integrated licensing.

**Template Features:**
- Multi-language support (Go 1.23.x, Python 3.12/3.13, Node.js 18+)
- Enterprise security and licensing integration
- Comprehensive CI/CD pipeline
- Production-ready containerization
- Monitoring and observability
- Version management system
- PenguinTech License Server integration

## Technology Stack

### Languages & Frameworks

**Language Selection Criteria (Case-by-Case Basis):**
- **Python 3.13**: Default choice for most applications
  - Web applications and APIs
  - Business logic and data processing
  - Integration services and connectors
- **Go 1.23.x**: ONLY for high-traffic/performance-critical applications
  - Applications handling >10K requests/second
  - Network-intensive services
  - Low-latency requirements (<10ms)
  - CPU-bound operations requiring maximum throughput

**Python Stack:**
- **Python**: 3.13 for all applications (3.12+ minimum)
- **Web Framework**: Flask + Flask-Security-Too (mandatory)
- **Database ORM**: PyDAL (mandatory for all Python applications)
- **Performance**: Dataclasses with slots, type hints, async/await required

**Frontend Stack:**
- **React**: ReactJS for all frontend applications
- **Node.js**: 18+ for build tooling and React development
- **JavaScript/TypeScript**: Modern ES2022+ standards

**Go Stack (When Required):**
- **Go**: 1.23.x (latest patch version)
- **Database**: Use DAL with PostgreSQL/MySQL cross-support (e.g., GORM, sqlx)
- Use only for traffic-intensive applications

### Infrastructure & DevOps
- **Containers**: Docker with multi-stage builds, Docker Compose
- **Orchestration**: Kubernetes with Helm charts
- **Configuration Management**: Ansible for infrastructure automation
- **CI/CD**: GitHub Actions with comprehensive pipelines
- **Monitoring**: Prometheus metrics, Grafana dashboards
- **Logging**: Structured logging with configurable levels

### Databases & Storage
- **Primary**: PostgreSQL (default, configurable via `DB_TYPE` environment variable)
- **Cache**: Redis/Valkey with optional TLS and authentication
- **Database Abstraction Layers (DALs)**:
  - **Python**: PyDAL (mandatory for ALL Python applications)
    - Must support ALL PyDAL-supported databases by default
    - Special support for MariaDB Galera cluster requirements
    - `DB_TYPE` must match PyDAL connection string prefixes exactly
  - **Go**: GORM or sqlx (mandatory for cross-database support)
    - Must support PostgreSQL and MySQL/MariaDB
    - Stable, well-maintained library required
- **Migrations**: Automated schema management
- **Database Support**: Design for ALL PyDAL-supported databases from the start
- **MariaDB Galera Support**: Handle Galera-specific requirements (WSREP, auto-increment, transactions)

**Supported DB_TYPE Values (PyDAL prefixes)**:
- `postgres` / `postgresql` - PostgreSQL (default)
- `mysql` - MySQL/MariaDB
- `sqlite` - SQLite
- `mssql` - Microsoft SQL Server
- `oracle` - Oracle Database
- `db2` - IBM DB2
- `firebird` - Firebird
- `informix` - IBM Informix
- `ingres` - Ingres
- `cubrid` - CUBRID
- `sapdb` - SAP DB/MaxDB

### Security & Authentication
- **Flask-Security-Too**: Mandatory for all Flask applications
  - Role-based access control (RBAC)
  - User authentication and session management
  - Password hashing with bcrypt
  - Email confirmation and password reset
  - Two-factor authentication (2FA)
- **TLS**: Enforce TLS 1.2 minimum, prefer TLS 1.3
- **HTTP3/QUIC**: Utilize UDP with TLS for high-performance connections where possible
- **Authentication**: JWT and MFA (standard), mTLS where applicable
- **SSO**: SAML/OAuth2 SSO as enterprise-only features
- **Secrets**: Environment variable management
- **Scanning**: Trivy vulnerability scanning, CodeQL analysis
- **Code Quality**: All code must pass CodeQL security analysis

## PenguinTech License Server Integration

All projects integrate with the centralized PenguinTech License Server at `https://license.penguintech.io` for feature gating and enterprise functionality.

**IMPORTANT: License enforcement is ONLY enabled when project is marked as release-ready**
- Development phase: All features available, no license checks
- Release phase: License validation required, feature gating active

**License Key Format**: `PENG-XXXX-XXXX-XXXX-XXXX-ABCD`

**Core Endpoints**:
- `POST /api/v2/validate` - Validate license
- `POST /api/v2/features` - Check feature entitlements
- `POST /api/v2/keepalive` - Report usage statistics

**Environment Variables**:
```bash
# License configuration
LICENSE_KEY=PENG-XXXX-XXXX-XXXX-XXXX-ABCD
LICENSE_SERVER_URL=https://license.penguintech.io
PRODUCT_NAME=your-product-identifier

# Release mode (enables license enforcement)
RELEASE_MODE=false  # Development (default)
RELEASE_MODE=true   # Production (explicitly set)
```

## WaddleAI Integration (Optional)

For projects requiring AI capabilities, integrate with WaddleAI located at `~/code/WaddleAI`.

**When to Use WaddleAI:**
- Natural language processing (NLP)
- Machine learning model inference
- AI-powered features and automation
- Intelligent data analysis
- Chatbots and conversational interfaces

**Integration Pattern:**
- WaddleAI runs as separate microservice container
- Communicate via REST API or gRPC
- Environment variable configuration for API endpoints
- License-gate AI features as enterprise functionality

### Authentication

All API calls use Bearer token authentication with the license key:
```bash
Authorization: Bearer PENG-XXXX-XXXX-XXXX-XXXX-ABCD
```

### Python Client Example

```python
from shared.licensing import PenguinTechLicenseClient, requires_feature

# Initialize client
client = PenguinTechLicenseClient(
    license_key=os.getenv('LICENSE_KEY'),
    product=os.getenv('PRODUCT_NAME')
)

# Validate license
validation = client.validate()
if validation.get("valid"):
    print(f"License valid for {validation['customer']} ({validation['tier']})")

# Feature gating decorator
@requires_feature("advanced_analytics")
def generate_report():
    """Requires professional+ license"""
    return analytics.generate_report()
```

## Project Structure

```
project-name/
├── .github/
│   ├── workflows/           # CI/CD pipelines
│   ├── ISSUE_TEMPLATE/      # Issue templates
│   └── PULL_REQUEST_TEMPLATE.md
├── apps/                    # Application code
│   ├── api/                 # API services (Go/Python)
│   ├── web/                 # Web applications (Python/Node.js)
│   └── cli/                 # CLI tools (Go)
├── services/                # Microservices
│   ├── service-name/
│   │   ├── cmd/             # Go main packages
│   │   ├── internal/        # Private application code
│   │   ├── pkg/             # Public library code
│   │   ├── Dockerfile       # Service container
│   │   └── go.mod           # Go dependencies
├── shared/                  # Shared components
│   ├── auth/                # Authentication utilities
│   ├── config/              # Configuration management
│   ├── database/            # Database utilities
│   ├── licensing/           # License server integration
│   ├── monitoring/          # Metrics and logging
│   └── types/               # Shared types/schemas
├── web/                     # Frontend applications
│   ├── public/              # Static assets
│   ├── src/                 # Source code
│   ├── package.json         # Node.js dependencies
│   └── Dockerfile           # Web container
├── infrastructure/          # Infrastructure as code
│   ├── docker/              # Docker configurations
│   ├── k8s/                 # Kubernetes manifests
│   ├── helm/                # Helm charts
│   └── monitoring/          # Prometheus/Grafana configs
├── scripts/                 # Utility scripts
│   ├── build/               # Build automation
│   ├── deploy/              # Deployment scripts
│   ├── test/                # Testing utilities
│   └── version/             # Version management
├── tests/                   # Test suites
│   ├── unit/                # Unit tests
│   ├── integration/         # Integration tests
│   ├── e2e/                 # End-to-end tests
│   └── performance/         # Performance tests
├── docs/                    # Documentation
│   ├── api/                 # API documentation
│   ├── deployment/          # Deployment guides
│   ├── development/         # Development setup
│   ├── licensing/           # License integration guide
│   ├── architecture/        # System architecture
│   └── RELEASE_NOTES.md     # Version release notes (prepend new releases)
├── config/                  # Configuration files
│   ├── development/         # Dev environment configs
│   ├── production/          # Production configs
│   └── testing/             # Test environment configs
├── docker-compose.yml       # Development environment
├── docker-compose.prod.yml  # Production environment
├── Makefile                 # Build automation
├── go.mod                   # Go workspace
├── requirements.txt         # Python dependencies
├── package.json             # Node.js workspace
├── .version                 # Version tracking
├── VERSION.md               # Versioning guidelines
├── README.md                # Project documentation
├── CONTRIBUTING.md          # Contribution guidelines
├── SECURITY.md              # Security policies
├── LICENSE.md               # License information
└── CLAUDE.md                # This file
```

## Version Management System

### Format: vMajor.Minor.Patch.build
- **Major**: Breaking changes, API changes, removed features
- **Minor**: Significant new features and functionality additions
- **Patch**: Minor updates, bug fixes, security patches
- **Build**: Epoch64 timestamp of build time (used between releases for automatic chronological ordering)

### Version Update Process
```bash
# Update version using provided scripts
./scripts/version/update-version.sh          # Increment build timestamp
./scripts/version/update-version.sh patch    # Increment patch version
./scripts/version/update-version.sh minor    # Increment minor version
./scripts/version/update-version.sh major    # Increment major version
./scripts/version/update-version.sh 1 2 3    # Set specific version
```

### Version Integration
- Embedded in applications and API responses
- Docker images tagged with full version for dev, semantic for releases
- Automated version bumping in CI/CD pipeline
- Version validation in build processes

## Development Workflow

### Local Development Setup
```bash
# Clone and setup
git clone <repository-url>
cd project-name
make setup                    # Install dependencies and setup environment
make dev                      # Start development environment
```

### Essential Commands
```bash
# Development
make dev                      # Start development services
make test                     # Run all tests
make lint                     # Run linting and code quality checks
make build                    # Build all services
make clean                    # Clean build artifacts

# Production
make docker-build             # Build production containers
make docker-push              # Push to registry
make deploy-dev               # Deploy to development
make deploy-prod              # Deploy to production

# Testing
make test-unit               # Run unit tests
make test-integration        # Run integration tests
make test-e2e                # Run end-to-end tests
make test-performance        # Run performance tests

# License Management
make license-validate        # Validate license configuration
make license-check-features  # Check available features
```

## Security Requirements

### Input Validation
- ALL inputs MUST have appropriate validators
- Use framework-native validation (pydal validators, Go validation libraries)
- Implement XSS and SQL injection prevention
- Server-side validation for all client input
- CSRF protection using framework native features

### Authentication & Authorization
- Multi-factor authentication support
- Role-based access control (RBAC)
- API key management with rotation
- JWT token validation with proper expiration
- Session management with secure cookies

### Security Scanning
- Automated dependency vulnerability scanning
- Container image security scanning
- Static code analysis for security issues
- Regular security audit logging
- Secrets scanning in CI/CD pipeline

## Enterprise Features

### Licensing Integration
- PenguinTech License Server integration
- Feature gating based on license tiers
- Usage tracking and reporting
- Compliance audit logging
- Enterprise support escalation

### Multi-Tenant Architecture
- Customer isolation and data segregation
- Per-tenant configuration management
- Usage-based billing integration
- White-label capabilities
- Compliance reporting (SOC2, ISO27001)

### Monitoring & Observability
- Prometheus metrics collection
- Grafana dashboards for visualization
- Structured logging with correlation IDs
- Distributed tracing support
- Real-time alerting and notifications

## CI/CD Pipeline Features

### Testing Pipeline
- Multi-language testing (Go, Python, Node.js)
- Parallel test execution for performance
- Code coverage reporting
- Security scanning integration
- Performance regression testing

### Build Pipeline
- **Multi-architecture Docker builds** (amd64/arm64) using separate parallel workflows
- **Debian-slim base images** for all container builds to minimize size and attack surface
- **Parallel workflow execution** to minimize total build time without removing functionality
- **Optimized build times**: Prioritize speed while maintaining full functionality
- Dependency caching for faster builds
- Artifact management and versioning
- Container registry integration
- Build optimization and layer caching

### Deployment Pipeline
- Environment-specific deployment configs
- Blue-green deployment support
- Rollback capabilities
- Health check validation
- Automated database migrations

### Quality Gates
- Required code review process
- Automated testing requirements
- Security scan pass requirements
- Performance benchmark validation
- Documentation update verification

## CI/CD Pipeline & .WORKFLOW Compliance

### Version Management Automation

The Nest project implements comprehensive version tracking with `.WORKFLOW` compliance:

**Version File Monitoring (version-monitor.yml)**
- Triggers on `.version` file changes
- Validates semantic versioning format (vMajor.Minor.Patch.build)
- Checks Epoch64 timestamp for build identification
- Ensures version consistency across files
- Validates builds with current version
- Performs security scanning in version context

**Version Release Process (version-release.yml)**
- Automatically creates GitHub releases when `.version` changes
- Generates comprehensive release notes
- Prevents duplicate releases
- Skips default versions (0.0.0)
- Tags commits with version information

### Comprehensive Security Scanning

**Multi-Language Security Tools:**
- **gosec**: Go security scanning (SARIF format output)
  - Detects hardcoded credentials, SQL injection risks, weak crypto
  - Repository: github.com/securego/gosec/v2
- **bandit**: Python vulnerability scanning
  - Identifies insecure deserialization, hardcoded secrets, insecure tempfiles
- **npm audit**: Node.js dependency vulnerability analysis
  - Scans package.json and package-lock.json
  - Supports audit-level severity filtering

**Integration Security:**
- **Trivy**: Filesystem vulnerability scanning (container images, dependencies)
- **CodeQL**: Semantic code analysis for Go, Python, JavaScript
- **Semgrep**: Pattern-based security policy enforcement

### Workflow Structure

**Primary Workflows:**

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| ci.yml | Push/PR | Multi-language testing, linting, security scanning |
| version-monitor.yml | .version changes | Version validation and consistency |
| version-release.yml | .version push to main | Automated GitHub release creation |
| push.yml | Push to main | Docker image build and publish |
| release.yml | GitHub release published | Release-triggered operations |
| cron.yml | Daily 2 AM UTC | Scheduled maintenance and dependency checks |

**Test Coverage Requirements:**
- Go: 80%+ coverage with race detector
- Python: 80%+ coverage with pytest
- Node.js: 80%+ coverage with Jest
- Integration tests across all components
- E2E tests for critical user flows

### Multi-Language Compiler Configuration

**Go Build System:**
- Version: 1.23.5+ (tested on 1.23.5, 1.24.0)
- Race detector enabled for all test runs
- Static analysis via staticcheck and gosec
- Module caching for faster builds
- Multi-version testing in CI

**Python Build System:**
- Versions: 3.12, 3.13
- Lint checks: black, isort, flake8, mypy
- Test framework: pytest with coverage
- Security: bandit scanning
- Services: PostgreSQL 15, Redis 7 for integration tests

**Node.js Build System:**
- Versions: 18, 20, 22
- Linting: ESLint + Prettier
- Type checking: TypeScript
- Testing: Jest with coverage
- Build output: SPA with dist/ artifacts

### Environment Variable Management

**CI/CD Standard Environment Variables:**
```yaml
GO_VERSION: '1.23.5'
PYTHON_VERSION: '3.12'
NODE_VERSION: '18'
REGISTRY: ghcr.io
```

**Test Environment Variables:**
```bash
# Database
DATABASE_URL: postgresql://test_user:test_pass@localhost:5432/test_db
REDIS_URL: redis://localhost:6379/1

# Licensing
LICENSE_KEY: PENG-TEST-TEST-TEST-TEST-ABCD
PRODUCT_NAME: test-product

# Application
RELEASE_MODE: false
```

### Deployment Workflow Standards

**Development Deployment:**
- From develop branch
- Runs all tests before deployment
- Uses development configuration
- No production data access

**Production Deployment:**
- Only from main/release branches
- Requires passing release workflow
- Manual approval gate required
- Change documentation mandatory
- Automated rollback capability

### Dependency Management

**Automated Scanning:**
- Dependabot alerts for pull requests
- GitHub security advisories monitored
- Weekly dependency update checks
- CVE vulnerability scanning

**Update Policy:**
- Critical/High vulnerabilities: Immediate update
- Medium vulnerabilities: Update within 1 week
- Low vulnerabilities: Update within 1 month
- Regular version updates (quarterly)

### Monitoring & Observability

**Metrics Collection:**
- Prometheus metrics endpoint: `/metrics`
- Health checks: `/health`, `/healthz`
- Coverage reports: Codecov integration
- Performance tracking: Build time trending

**Logging Strategy:**
- Structured logging (JSON format)
- Multiple severity levels (DEBUG, INFO, WARNING, ERROR)
- Correlation IDs for request tracing
- Audit logs for security events

### Documentation Reference

For detailed information, see:
- **docs/WORKFLOWS.md**: Complete workflow documentation
- **docs/STANDARDS.md**: Development standards and compliance requirements

## Critical Development Rules

### Development Philosophy: Safe, Stable, and Feature-Complete

**NEVER take shortcuts or the "easy route" - ALWAYS prioritize safety, stability, and feature completeness**

#### Core Principles
- **No Quick Fixes**: Resist quick workarounds or partial solutions
- **Complete Features**: Fully implemented with proper error handling and validation
- **Safety First**: Security, data integrity, and fault tolerance are non-negotiable
- **Stable Foundations**: Build on solid, tested components
- **Future-Proof Design**: Consider long-term maintainability and scalability
- **No Technical Debt**: Address issues properly the first time

#### Red Flags (Never Do These)
- Skipping input validation "just this once"
- Hardcoding credentials or configuration
- Ignoring error returns or exceptions
- Commenting out failing tests to make CI pass
- Deploying without proper testing
- Using deprecated or unmaintained dependencies
- Implementing partial features with "TODO" placeholders
- Bypassing security checks for convenience
- Assuming data is valid without verification
- Leaving debug code or backdoors in production

#### Quality Checklist Before Completion
- All error cases handled properly
- Unit tests cover all code paths
- Integration tests verify component interactions
- Security requirements fully implemented
- Performance meets acceptable standards
- Documentation complete and accurate
- Code review standards met
- No hardcoded secrets or credentials
- Logging and monitoring in place
- Build passes in containerized environment
- No security vulnerabilities in dependencies
- Edge cases and boundary conditions tested

### Git Workflow
- **NEVER commit automatically** unless explicitly requested by the user
- **NEVER push to remote repositories** under any circumstances
- **ONLY commit when explicitly asked** - never assume commit permission
- Always use feature branches for development
- Require pull request reviews for main branch
- Automated testing must pass before merge

### Local State Management (Crash Recovery)
- **ALWAYS maintain local .PLAN and .TODO files** for crash recovery
- **Keep .PLAN file updated** with current implementation plans and progress
- **Keep .TODO file updated** with task lists and completion status
- **Update these files in real-time** as work progresses
- **Add to .gitignore**: Both .PLAN and .TODO files must be in .gitignore
- **File format**: Use simple text format for easy recovery
- **Automatic recovery**: Upon restart, check for existing files to resume work

### Dependency Security Requirements
- **ALWAYS check for Dependabot alerts** before every commit
- **Monitor vulnerabilities via Socket.dev** for all dependencies
- **Mandatory security scanning** before any dependency changes
- **Fix all security alerts immediately** - no commits with outstanding vulnerabilities
- **Regular security audits**: `npm audit`, `go mod audit`, `safety check`

### Linting & Code Quality Requirements
- **ALL code must pass linting** before commit - no exceptions
- **Python**: flake8, black, isort, mypy (type checking), bandit (security)
- **JavaScript/TypeScript**: ESLint, Prettier
- **Go**: golangci-lint (includes staticcheck, gosec, etc.)
- **Ansible**: ansible-lint
- **Docker**: hadolint
- **YAML**: yamllint
- **Markdown**: markdownlint
- **Shell**: shellcheck
- **CodeQL**: All code must pass CodeQL security analysis
- **PEP Compliance**: Python code must follow PEP 8, PEP 257 (docstrings), PEP 484 (type hints)

### Build & Deployment Requirements
- **NEVER mark tasks as completed until successful build verification**
- All Go and Python builds MUST be executed within Docker containers
- Use containerized builds for local development and CI/CD pipelines
- Build failures must be resolved before task completion

### Documentation Standards
- **README.md**: Keep as overview and pointer to comprehensive docs/ folder
- **docs/ folder**: Create comprehensive documentation for all aspects
- **RELEASE_NOTES.md**: Maintain in docs/ folder, prepend new version releases to top
- Update CLAUDE.md when adding significant context
- **Build status badges**: Always include in README.md
- **ASCII art**: Include catchy, project-appropriate ASCII art in README
- **Company homepage**: Point to www.penguintech.io
- **License**: All projects use Limited AGPL3 with preamble for fair use

### File Size Limits
- **Maximum file size**: 25,000 characters for ALL code and markdown files
- **Split large files**: Decompose into modules, libraries, or separate documents
- **CLAUDE.md exception**: Maximum 39,000 characters (only exception to 25K rule)
- **Documentation strategy**: Create detailed documentation in `docs/` folder and link from CLAUDE.md
- **User approval required**: ALWAYS ask user permission before splitting CLAUDE.md files
- **Use Task Agents**: Utilize task agents (subagents) for expedient handling of large file changes

### Docker Build Standards
```bash
# Go builds within containers (using debian-slim)
docker run --rm -v $(pwd):/app -w /app golang:1.23-slim go build -o bin/app
docker build -t app:latest .

# Python builds within containers (using debian-slim)
# Use Python 3.12 for py4web applications due to py4web compatibility issues with 3.13
docker run --rm -v $(pwd):/app -w /app python:3.12-slim pip install -r requirements.txt
docker build -t web:latest .

# Use multi-stage builds with debian-slim for optimized production images
FROM golang:1.23-slim AS builder
FROM debian:stable-slim AS runtime

FROM python:3.12-slim AS builder
FROM debian:stable-slim AS runtime
```

### GitHub Actions Multi-Arch Build Strategy
```yaml
# Single workflow with multi-arch builds for each container
name: Build Containers
jobs:
  build-app:
    runs-on: ubuntu-latest
    steps:
      - uses: docker/build-push-action@v4
        with:
          platforms: linux/amd64,linux/arm64
          context: ./apps/app
          file: ./apps/app/Dockerfile

  build-manager:
    runs-on: ubuntu-latest
    steps:
      - uses: docker/build-push-action@v4
        with:
          platforms: linux/amd64,linux/arm64
          context: ./apps/manager
          file: ./apps/manager/Dockerfile

# Separate parallel workflows for each container type (app, manager, etc.)
# Each workflow builds multi-arch for that specific container
# Minimize build time through parallel container builds and caching
```

### Code Quality
- Follow language-specific style guides
- Comprehensive test coverage (80%+ target)
- No hardcoded secrets or credentials
- Proper error handling and logging
- Security-first development approach

### Unit Testing Requirements
- **All applications MUST have comprehensive unit tests**
- **Network isolation**: Unit tests must NOT require external network connections
- **No external dependencies**: Cannot reach databases, APIs, or external services
- **Use mocks/stubs**: Mock all external dependencies and I/O operations
- **KISS principle**: Keep unit tests simple, focused, and fast
- **Test isolation**: Each test should be independent and repeatable
- **Fast execution**: Unit tests should complete in milliseconds, not seconds

### Performance Best Practices
- **Always implement async/concurrent patterns** to maximize CPU and memory utilization
- **Python**: Use asyncio, threading, multiprocessing where appropriate
  - **Modern Python optimizations**: Leverage dataclasses, typing, and memory-efficient features from Python 3.12+
  - **Dataclasses**: Use @dataclass for structured data to reduce memory overhead and improve performance
  - **Type hints**: Use comprehensive typing for better optimization and IDE support
  - **Advanced features**: Utilize slots, frozen dataclasses, and other memory-efficient patterns
- **Go**: Leverage goroutines, channels, and the Go runtime scheduler
- **Networking Applications**: Implement high-performance networking optimizations:
  - eBPF/XDP for kernel-level packet processing and filtering
  - AF_XDP for high-performance user-space packet processing
  - NUMA-aware memory allocation and CPU affinity
  - Zero-copy networking techniques where applicable
  - Connection pooling and persistent connections
  - Load balancing with CPU core pinning
- **Memory Management**: Optimize for cache locality and minimize allocations
- **I/O Operations**: Use non-blocking I/O, buffering, and batching strategies
- **Database Access**: Implement connection pooling, prepared statements, and query optimization

### Documentation
- **README.md**: Keep as overview and pointer to comprehensive docs/ folder
- **docs/ folder**: Create comprehensive documentation for all aspects
- **RELEASE_NOTES.md**: Maintain in docs/ folder, prepend new version releases to top
- Update CLAUDE.md when adding significant context
- API documentation must be comprehensive
- Architecture decisions should be documented
- Security procedures must be documented

### README.md Standards
- **ALWAYS include build status badges** at the top of every README.md:
  - CI/CD pipeline status (GitHub Actions)
  - Test coverage status (Codecov)
  - Go Report Card (for Go projects)
  - Version badge
  - License badge (Limited AGPL3 with preamble for fair use)
- **ALWAYS include catchy ASCII art** below the build status badges
  - Use project-appropriate ASCII art that reflects the project's identity
  - Keep ASCII art clean and professional
  - Place in code blocks for proper formatting
- **Company homepage reference**: All project READMEs and sales websites should point to **www.penguintech.io** as the company's homepage
- **License standard**: All projects use Limited AGPL3 with preamble for fair use, not MIT

### CLAUDE.md File Management
- **Primary file**: Maintain main CLAUDE.md at project root
- **Split files when necessary**: For large/complex projects, create app-specific CLAUDE.md files
- **File structure for splits**:
  - `projectroot/CLAUDE.md` - Main context and cross-cutting concerns
  - `projectroot/app-folder/CLAUDE.md` - App-specific context and instructions
- **Root file linking**: Main CLAUDE.md should reference and link to app-specific files
- **User approval required**: ALWAYS ask user permission before splitting CLAUDE.md files
- **Split criteria**: Only split for genuinely large situations where single file becomes unwieldy

### Application Architecture Requirements

#### Web Framework Standards
- **py4web primary**: Use py4web for ALL application web structures (sales/docs websites exempt)
- **Health endpoints**: ALL applications must implement `/healthz` endpoint
- **Metrics endpoints**: ALL applications must implement Prometheus metrics endpoint using py4web

#### Logging & Monitoring
- **Console logging**: Always implement console output
- **Multi-destination logging**: Support multiple log destinations:
  - UDP syslog to remote log collection servers (legacy)
  - HTTP3/QUIC to Kafka clusters for high-performance log streaming
  - Cloud-native logging services (AWS CloudWatch, GCP Cloud Logging) via HTTP3
- **Logging levels**: Implement standardized verbosity levels:
  - `-v`: Warnings and criticals only
  - `-vv`: Info level (default)
  - `-vvv`: Debug logging
- **getopts**: Use Python getopts library instead of params where possible

#### Database & Caching Standards
- **PostgreSQL default**: Default to PostgreSQL with non-root user/password and dedicated database
- **PyDAL usage**: Only use PyDAL for databases with full PyDAL support
- **Redis/Valkey**: Utilize Redis/Valkey with optional TLS and authentication where appropriate

#### Security Implementation
- **TLS enforcement**: Enforce TLS 1.2 minimum, prefer TLS 1.3
- **Connection security**: Use HTTPS connections where possible, WireGuard where HTTPS not available
- **Modern logging transport**: HTTP3/QUIC for Kafka and cloud logging services (AWS/GCP)
- **Legacy syslog**: UDP syslog maintained for compatibility
- **Standard security**: Implement JWT, MFA, and mTLS in all versions where applicable
- **Enterprise SSO**: SAML/OAuth2 SSO as enterprise-only features
- **HTTP3/QUIC**: Use UDP with TLS for high-performance connections where possible

### Ansible Integration Requirements
- **Documentation Research**: ALWAYS research Ansible modules on https://docs.ansible.com before implementation
- **Module verification**: Check official documentation for:
  - Correct module names and syntax
  - Required and optional parameters
  - Return values and data structures
  - Version compatibility and requirements
- **Best practices**: Follow Ansible community standards and idempotency principles
- **Testing**: Ensure playbooks are idempotent and properly handle error conditions

### Website Integration Requirements
- **Each project MUST have two dedicated websites**:
  - Marketing/Sales website (Node.js based)
  - Documentation website (Markdown based)
- **Website Design Preferences**:
  - **Multi-page design preferred** - avoid single-page applications for marketing sites
  - **Modern aesthetic** with clean, professional appearance
  - **Not overly bright** - use subtle, sophisticated color schemes
  - **Gradient usage encouraged** - subtle gradients for visual depth and modern appeal
  - **Responsive design** - must work seamlessly across all device sizes
  - **Performance focused** - fast loading times and optimized assets
- **Website Repository Integration**:
  - Add `github.com/penguintechinc/website` as a sparse checkout submodule
  - Only include the project's specific website folders in the sparse checkout
  - Folder naming convention:
    - `{app_name}/` - Marketing and sales website
    - `{app_name}-docs/` - Documentation website
- **Sparse Submodule Setup**:
  ```bash
  # First, check if folders exist in the website repo and create if needed
  git clone https://github.com/penguintechinc/website.git temp-website
  cd temp-website

  # Create project folders if they don't exist
  mkdir -p {app_name}/
  mkdir -p {app_name}-docs/

  # Create initial template files if folders are empty
  if [ ! -f {app_name}/package.json ]; then
    # Initialize Node.js marketing website
    echo "Creating initial marketing website structure..."
    # Add basic package.json, index.js, etc.
  fi

  if [ ! -f {app_name}-docs/README.md ]; then
    # Initialize documentation website
    echo "Creating initial docs website structure..."
    # Add basic markdown structure
  fi

  # Commit and push if changes were made
  git add .
  git commit -m "Initialize website folders for {app_name}"
  git push origin main
  cd .. && rm -rf temp-website

  # Now add sparse submodule for website integration
  git submodule add --name websites https://github.com/penguintechinc/website.git websites
  git config -f .gitmodules submodule.websites.sparse-checkout true

  # Configure sparse checkout to only include project folders
  echo "{app_name}/" > .git/modules/websites/info/sparse-checkout
  echo "{app_name}-docs/" >> .git/modules/websites/info/sparse-checkout

  # Initialize sparse checkout
  git submodule update --init websites
  ```
- **Website Maintenance**: Both websites must be kept current with project releases and feature updates
- **First-Time Setup**: If project folders don't exist in the website repo, they must be created and initialized with basic templates before setting up the sparse submodule

## Application Architecture

**ALWAYS use microservices architecture** - decompose into specialized, independently deployable containers:

1. **Web UI Container**: ReactJS frontend (separate container, served via nginx)
2. **Application API Container**: Flask + Flask-Security-Too backend (separate container)
3. **Connector Container**: External system integration (separate container)

**Default Container Separation**: Web UI and API are ALWAYS separate containers by default. This provides:
- Independent scaling of frontend and backend
- Different resource allocation per service
- Separate deployment lifecycles
- Technology-specific optimization

**Benefits**:
- Independent scaling
- Technology diversity
- Team autonomy
- Resilience
- Continuous deployment

## Common Integration Patterns

### Flask + Flask-Security-Too + PyDAL
```python
from flask import Flask
from flask_security import Security, auth_required
from pydal import DAL, Field
import os

app = Flask(__name__)
app.config['SECRET_KEY'] = os.getenv('SECRET_KEY')
app.config['SECURITY_PASSWORD_SALT'] = os.getenv('SECURITY_PASSWORD_SALT')

# PyDAL database connection
db = DAL(
    f"postgresql://{os.getenv('DB_USER')}:{os.getenv('DB_PASS')}@"
    f"{os.getenv('DB_HOST')}:{os.getenv('DB_PORT')}/{os.getenv('DB_NAME')}",
    pool_size=10
)

# Define tables with PyDAL
db.define_table('users',
    Field('email', 'string', requires=IS_EMAIL(), unique=True),
    Field('password', 'string'),
    Field('active', 'boolean', default=True),
    Field('fs_uniquifier', 'string', unique=True),
    migrate=True)

# Flask-Security-Too setup
from flask_security import PyDALUserDatastore
user_datastore = PyDALUserDatastore(db, db.users, db.roles)
security = Security(app, user_datastore)

@app.route('/api/protected')
@auth_required()
def protected_resource():
    return {'message': 'This is a protected endpoint'}

@app.route('/healthz')
def health():
    return {'status': 'healthy'}, 200
```

### ReactJS Frontend Integration
```javascript
// API client for Flask backend
import axios from 'axios';

const API_BASE_URL = process.env.REACT_APP_API_URL || 'http://localhost:5000';

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add auth token to requests
apiClient.interceptors.request.use((config) => {
  const token = localStorage.getItem('authToken');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Protected component example
import React, { useEffect, useState } from 'react';

function ProtectedComponent() {
  const [data, setData] = useState(null);

  useEffect(() => {
    apiClient.get('/api/protected')
      .then(response => setData(response.data))
      .catch(error => console.error('Error:', error));
  }, []);

  return <div>{data?.message}</div>;
}
```

### License-Gated Features
```python
# Python feature gating
from shared.licensing import license_client, requires_feature
from flask_security import auth_required

@app.route('/api/advanced/analytics')
@auth_required()
@requires_feature("advanced_analytics")
def generate_advanced_report():
    """Requires authentication AND professional+ license"""
    return {'report': analytics.generate_report()}

# Startup validation
def initialize_application():
    client = license_client.get_client()
    validation = client.validate()
    if not validation.get("valid"):
        logger.error(f"License validation failed: {validation.get('message')}")
        sys.exit(1)

    logger.info(f"License valid for {validation['customer']} ({validation['tier']})")
    return validation
```

```go
// Go feature gating
package main

import (
    "log"
    "os"
    "your-project/internal/license"
)

func main() {
    client := license.NewClient(os.Getenv("LICENSE_KEY"), "your-product")

    validation, err := client.Validate()
    if err != nil || !validation.Valid {
        log.Fatal("License validation failed")
    }

    log.Printf("License valid for %s (%s)", validation.Customer, validation.Tier)

    // Check features
    if hasAdvanced, _ := client.CheckFeature("advanced_feature"); hasAdvanced {
        log.Println("Advanced features enabled")
    }
}
```

### Database Integration
```python
# Python with PyDAL
from pydal import DAL, Field

db = DAL('postgresql://user:pass@host/db')
db.define_table('users',
    Field('name', 'string', requires=IS_NOT_EMPTY()),
    Field('email', 'string', requires=IS_EMAIL()),
    migrate=True, fake_migrate=False)
```

```go
// Go with GORM
import "gorm.io/gorm"

type User struct {
    ID    uint   `gorm:"primaryKey"`
    Name  string `gorm:"not null"`
    Email string `gorm:"uniqueIndex;not null"`
}
```

### API Development
```python
# Python with py4web
from py4web import action, request, response
from py4web.utils.cors import CORS

@action('api/users', method=['GET', 'POST'])
@CORS()
def api_users():
    if request.method == 'GET':
        return {'users': db(db.users).select().as_list()}
    # Handle POST...
```

```go
// Go with Gin
func setupRoutes() *gin.Engine {
    r := gin.Default()
    r.Use(cors.Default())

    v1 := r.Group("/api/v1")
    {
        v1.GET("/users", getUsers)
        v1.POST("/users", createUser)
    }
    return r
}
```

### Monitoring Integration
```python
# Python metrics
from prometheus_client import Counter, Histogram, generate_latest

REQUEST_COUNT = Counter('http_requests_total', 'Total HTTP requests', ['method', 'endpoint'])
REQUEST_DURATION = Histogram('http_request_duration_seconds', 'HTTP request duration')

@action('metrics')
def metrics():
    return generate_latest(), {'Content-Type': 'text/plain'}
```

```go
// Go metrics
import "github.com/prometheus/client_golang/prometheus"

var (
    requestCount = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "http_requests_total"},
        []string{"method", "endpoint"})
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "http_request_duration_seconds"},
        []string{"method", "endpoint"})
)
```

## Troubleshooting & Support

### Common Issues
1. **Port Conflicts**: Check docker-compose port mappings
2. **Database Connections**: Verify connection strings and permissions
3. **License Validation Failures**: Check license key format and network connectivity
4. **Build Failures**: Check dependency versions and compatibility
5. **Test Failures**: Review test environment setup

### Debug Commands
```bash
# Container debugging
docker-compose logs -f service-name
docker exec -it container-name /bin/bash

# Application debugging
make debug                    # Start with debug flags
make logs                     # View application logs
make health                   # Check service health

# License debugging
make license-debug            # Test license server connectivity
make license-validate         # Validate current license
```

### License Server Support
- **Technical Documentation**: Complete API reference available
- **Integration Support**: support@penguintech.io
- **Sales Inquiries**: sales@penguintech.io
- **License Server Status**: https://status.penguintech.io

## Template Customization

### Adding New Languages
1. Create language-specific directory structure
2. Add Dockerfile and build scripts
3. Update CI/CD pipeline configuration
4. Add language-specific linting and testing
5. Update documentation and examples

### Adding New Services
1. Use service template in `services/` directory
2. Configure service discovery and networking
3. Add monitoring and logging integration
4. Integrate license checking for service features
5. Create service-specific tests
6. Update deployment configurations

### Enterprise Integration
- Configure license server integration
- Set up multi-tenant data isolation
- Implement usage tracking and reporting
- Add compliance audit logging
- Configure enterprise monitoring

---

**Template Version**: 1.2.0
**Last Updated**: 2025-11-23
**Maintained by**: Penguin Tech Inc
**License Server**: https://license.penguintech.io

**Key Updates in v1.2.0:**
- Web UI and API as separate containers by default
- Mandatory linting for all languages (flake8, ansible-lint, eslint, etc.)
- CodeQL inspection compliance required
- Multi-database support by design (all PyDAL databases + MariaDB Galera)
- DB_TYPE environment variable with input validation
- Flask as sole web framework (PyDAL for database abstraction)

**Key Updates in v1.1.0:**
- Flask-Security-Too mandatory for authentication
- ReactJS as standard frontend framework
- Python 3.13 vs Go decision criteria
- WaddleAI integration patterns
- Release-mode license enforcement

*Production-ready foundation for enterprise software development with comprehensive tooling, security, and licensing.*