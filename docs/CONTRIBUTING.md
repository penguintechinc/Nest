# Contributing to Nest

Welcome! This guide covers everything you need to contribute to Nest, a Kubernetes-native unified storage and data platform.

## Prerequisites

Ensure you have the following installed and configured:

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.24.2+ | Services (k8s-controller, node-agent) |
| Python | 3.13+ | API service (apps/api) |
| Node.js | 24+ | Admin UI (web/) |
| Docker | Latest | Container builds |
| kubectl | 1.28+ | Kubernetes cluster management |
| Helm | 3.x | Deployment (beta/prod) |
| Rook-Ceph | Operational | CSI driver backend (required for any K8s deployment) |

**Cluster access:**
- Alpha: `local-alpha` context (MicroK8s or Docker Desktop)
- Beta: `dal2-beta` context
- Production: Environment-specific context

**Optional but recommended:**
- `microk8s` (Linux) or Docker Desktop (macOS) for local K8s
- `golangci-lint` (Go linting)
- `mypy` (Python type checking)
- `eslint` (JavaScript/TypeScript linting)

## Repository Structure

```
nest/
├── apis/v1/                      # CRDs & API types
│   ├── dataresource_types.go     # DataResource CRD definition
│   ├── compliance_types.go       # CompliancePolicy CRD
│   ├── storage_types.go          # StoragePool CRD
│   ├── label_types.go            # Labels CRD
│   ├── federation_types.go       # Federation CRD
│   └── zz_generated.deepcopy.go  # Generated (DO NOT EDIT)
├── services/
│   ├── k8s-controller/           # Kubernetes controller reconciler
│   │   ├── main.go
│   │   ├── controllers/          # Reconciliation logic
│   │   └── go.mod
│   └── node-agent/               # Node-level daemon
│       ├── agent.go
│       ├── inventory.go          # Hardware detection
│       └── go.mod
├── apps/
│   ├── api/                      # Python REST API
│   │   ├── app.py
│   │   ├── handlers/             # DataResource, probe, import handlers
│   │   ├── models.py
│   │   ├── requirements.txt
│   │   ├── pytest.ini
│   │   ├── tests/                # Unit + integration tests
│   │   └── Dockerfile
│   └── manager/                  # Manager component (TBD)
├── web/                          # React admin UI
│   ├── src/
│   │   ├── pages/                # DataResource, cluster, admin pages
│   │   ├── components/           # Shared UI components
│   │   └── api/                  # API client
│   ├── package.json
│   ├── package-lock.json
│   └── Dockerfile
├── tools/
│   └── nestctl/                  # CLI tool (TBD)
├── k8s/
│   ├── helm/                     # Helm charts for beta/prod
│   │   ├── nest/
│   │   │   ├── Chart.yaml
│   │   │   ├── values.yaml
│   │   │   ├── values-beta.yaml
│   │   │   ├── values-prod.yaml
│   │   │   └── templates/
│   │   └── nest-csi/
│   └── kustomize/                # Kustomize overlays for alpha
│       ├── base/                 # All services + dependencies
│       └── overlays/
│           ├── alpha/
│           ├── beta/
│           └── prod/
├── docs/
│   ├── spec/                     # Specification documents
│   │   └── storage-types.md      # DataResource type documentation
│   ├── ops/                      # Operational guides
│   │   ├── migrate-from-longhorn.md
│   │   └── object-storage-lifecycle.md
│   ├── migration/                # Migration tooling docs
│   │   └── longhorn-to-nest.md
│   ├── infrastructure/           # Ceph + cluster setup
│   ├── standards/                # Development standards
│   ├── USAGE.md
│   ├── WORKFLOWS.md
│   └── CONTRIBUTING.md
├── Makefile                      # Development commands
├── go.mod / go.sum               # Go dependencies
├── README.md                     # Project overview
└── .version                      # Semantic version + build epoch

cmd/                              # Command-line tools (TBD)
anomaly-detector/                 # Anomaly detection service (TBD)
audit-service/                    # Audit logging (TBD)
cost-calculator/                  # Cost analysis (TBD)
data-indexer/                     # Metadata indexing (TBD)
erasure-engine/                   # Erasure coding (TBD)
federation-controller/            # Cross-cluster federation (TBD)
iceberg-catalog/                  # Apache Iceberg integration (TBD)
injector/                         # Webhook injector (TBD)
intelligence-engine/              # ML-based optimization (TBD)
```

## Development Setup

### 1. Clone & Install Dependencies

```bash
git clone https://github.com/penguintechinc/nest.git
cd nest

# Install all dependencies
make setup

# Or manually:
go mod download && go mod tidy          # Go
pip install -r apps/api/requirements.txt  # Python
cd web && npm ci                        # Node.js
```

### 2. Local Kubernetes Cluster

Set up MicroK8s (Linux) or Docker Desktop (macOS) with a local registry:

```bash
# MicroK8s setup (Linux)
snap install microk8s --classic
microk8s enable registry dns metrics-server

# Docker Desktop (macOS/Windows)
# Enable in Docker Desktop → Settings → Kubernetes

# Create local-alpha context
kubectl config set-context local-alpha --cluster=microk8s-cluster --user=admin
# or
kubectl config rename-context docker-desktop local-alpha
```

### 3. Deploy Rook-Ceph (Required)

Before deploying Nest, initialize a Rook-Ceph cluster:

```bash
# Follow Rook-Ceph quick start
# https://rook.io/docs/rook/latest/Getting-Started/quickstart/

# Verify it's running
kubectl --context local-alpha get pods -n rook-ceph
```

### 4. Run Tests

```bash
# Run all tests
make test

# Go tests (unit + integration)
make test-go

# Python tests (apps/api)
make test-python

# Node.js tests (web)
make test-node

# K8s integration tests (against docker-desktop)
make test-k8s
```

### 5. Local Development Services

```bash
# Start full development stack (databases + services)
make dev-full

# Or start just databases
make dev-db

# And run services individually:
make dev-services     # Run API + web concurrently
make dev-api          # Go API service
make dev-web-python   # Python API (if separate)
make dev-web-node     # React UI dev server
```

### 6. Deploy to Alpha Cluster

```bash
# Build and push images to local registry
docker build -t localhost:32000/nest-api:latest apps/api/
docker push localhost:32000/nest-api:latest

docker build -t localhost:32000/nest-web:latest web/
docker push localhost:32000/nest-web:latest

# Deploy via Kustomize
kubectl apply -k k8s/kustomize/overlays/alpha --context local-alpha

# Verify
kubectl --context local-alpha get pods -n nest
```

## Code Style & Linting

### Go

```bash
# Format
go fmt ./...
goimports -w .

# Lint
make lint-go
# or
golangci-lint run

# Vet
go vet ./...
```

**Standards:**
- CamelCase for exported symbols
- kebab-case for file/directory names
- Comments for all exported functions
- Error handling mandatory (no ignored errors)

### Python

```bash
# Format & sort imports
black apps/api/
isort apps/api/

# Lint
make lint-python
# or
flake8 apps/api/
mypy apps/api/

# Type hints required
# mypy --strict must pass
```

**Standards:**
- snake_case for functions/variables
- Type hints on all functions
- Docstrings (PEP 257) for classes/modules
- 90%+ test coverage

### TypeScript/JavaScript (React)

```bash
# Format & lint
cd web
npm run lint
npm run format

# Type checking
npm run typecheck

# Make sure prettier passes
npx prettier --write .
```

**Standards:**
- camelCase for variables/functions
- PascalCase for components
- Arrow functions for components
- 90%+ test coverage
- No console.log in production (use structured logging)

## Testing Requirements

All code changes must include appropriate tests:

### Unit Tests

```bash
# Go
go test -v -race ./...

# Python
pytest apps/api/tests/unit/ -v

# JavaScript/TypeScript
cd web && npm test
```

### Integration Tests

```bash
# Go (requires running services)
go test -tags=integration -v ./tests/...

# Python (uses test DB)
pytest apps/api/tests/integration/ -v --cov

# Playwright/E2E
cd web && npm run test:e2e
```

### Coverage Requirements

- **Minimum 90%** across all languages (lines, branches, functions, statements)
- Run with coverage reports:
  ```bash
  make test-coverage
  ```
- Builds fail if coverage drops below 90%

## PR Process

### Branch Strategy

1. Create a feature/fix/chore branch from the **release branch** (e.g., `release/v1.0.X`), not `main`:

   ```bash
   git checkout -b feature/add-snapshot-policy origin/release/v1.0.X
   ```

2. Push and open a PR back to the same release branch

3. After PR merge, delete the feature branch

### PR Checklist

- [ ] All tests pass locally: `make test`
- [ ] Linting passes: `make lint`
- [ ] Code coverage ≥90%
- [ ] No TODOs, stubs, or incomplete implementations
- [ ] Database schema changes include Alembic migrations (Python API)
- [ ] CRD changes regenerate deepcopy and update docs (Go services)
- [ ] Documentation updated if user-facing behavior changed
- [ ] CHANGELOG entry added (for features/breaking changes)

### Security Pre-Commit

Before pushing, run security checks:

```bash
make security-scan
# Includes: gosec (Go), bandit (Python), npm audit (Node.js)
```

## DataResource Type Changes

When adding or modifying a `DataResource` type:

### 1. Update CRD Definition

Edit `/apis/v1/dataresource_types.go`:

```go
type DataResourceSpec struct {
    Type string `json:"type"` // e.g., "block", "filesystem", "object"
    // ... other fields
}
```

### 2. Regenerate Deepcopy

```bash
# Install if not present
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

# Generate deepcopy methods
controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./apis/..."

# Verify zz_generated.deepcopy.go was updated
git diff apis/v1/zz_generated.deepcopy.go
```

### 3. Update Documentation

Edit `/docs/spec/storage-types.md` with:
- Type name and description
- Valid state transitions
- Example YAML manifest
- Validation rules

### 4. Wire Controller Reconciler

Add handler in `/services/k8s-controller/controllers/dataresource_controller.go`:

```go
func (r *DataResourceReconciler) reconcileBlockStorage(ctx context.Context, dr *v1.DataResource) error {
    // Reconciliation logic
}
```

### 5. Add API Handler

Add endpoint in `/apps/api/handlers/dataresource.py`:

```python
@bp.post('/dataresources/<type>')
async def create_dataresource(type):
    # Handle new type
```

### 6. Add UI Page

Create React component in `/web/src/pages/` (e.g., `BlockStoragePage.tsx`)

### 7. Update Sidebar Navigation

Edit `/web/src/components/Sidebar.tsx` to include new type

### 8. Write Tests

- Controller reconciliation test
- API handler test
- Integration test
- Smoke test for UI page

## CRD & Controller Updates

### Running Controller Locally

```bash
# Build and run (requires kubectl access)
make build-go
cd services/k8s-controller
go run main.go

# Or use air for auto-reload
cd services/k8s-controller
air
```

### Testing Controller Reconciliation

```bash
# Unit tests
cd services/k8s-controller
go test -v ./controllers/

# Integration test against running cluster
make test-k8s
```

## API Handler Changes

### Python API (apps/api)

**Add a new endpoint:**

1. Create handler in `/apps/api/handlers/{resource}.py`:
   ```python
   from quart import Blueprint, jsonify, request
   
   bp = Blueprint('resource', __name__, url_prefix='/api/v1/resources')
   
   @bp.get('/')
   async def list_resources():
       # Implementation
   ```

2. Register in `app.py`:
   ```python
   from handlers import resource
   app.register_blueprint(resource.bp)
   ```

3. Add type definitions in `models.py`
4. Write tests in `tests/api/resource/`
5. Document in OpenAPI spec

**Database operations:**

- Use `penguin-dal` for all runtime queries
- Use `SQLAlchemy` + `Alembic` for schema migrations
- Per-service database accounts (principle of least privilege)

### Testing API Changes

```bash
# Run API tests
pytest apps/api/tests/api/ -v

# Test specific endpoint
pytest apps/api/tests/api/dataresource/ -v

# Coverage
pytest apps/api/tests/ --cov=apps/api --cov-report=html
```

## React UI Changes

### Adding a New Page

1. Create `/web/src/pages/{Feature}Page.tsx`
2. Add route to `/web/src/App.tsx`
3. Update sidebar in `/web/src/components/Sidebar.tsx`
4. Add data-testid attributes for testing
5. Use shared components from `@penguintechinc/react-libs`
6. Add unit + Playwright smoke tests

### Testing UI Changes

```bash
cd web

# Unit tests
npm test

# Type checking
npm run typecheck

# Lint & format
npm run lint
npm run format

# Smoke tests (requires running backend)
npm run test:e2e
```

## Kubernetes Manifest Changes

### Alpha (Kustomize)

Edit `/k8s/kustomize/overlays/alpha/`:

```bash
kubectl kustomize k8s/kustomize/overlays/alpha
kubectl apply -k k8s/kustomize/overlays/alpha --context local-alpha
```

### Beta/Prod (Helm)

Edit `/k8s/helm/nest/`:

```bash
helm dependency update k8s/helm/nest
helm lint k8s/helm/nest
helm upgrade --install nest k8s/helm/nest --context dal2-beta -f k8s/helm/nest/values-beta.yaml
```

**Always include:**
- Resource requests/limits
- Liveness + readiness probes
- securityContext (runAsNonRoot, readOnlyRootFilesystem)

## Documentation

All user-facing changes require documentation:

- **New feature?** Update `/docs/USAGE.md` or create new guide in `/docs/ops/`
- **API change?** Update OpenAPI spec or `/docs/api/`
- **Workflow change?** Update `/docs/WORKFLOWS.md`
- **Architecture change?** Update `/docs/standards/ARCHITECTURE.md`
- **New DataResource type?** Update `/docs/spec/storage-types.md`

## Commit Guidelines

- One feature/fix per commit
- Use imperative tense: "Add DataResource snapshot policy" not "Added..."
- Reference GitHub issue: `Closes #123`
- Keep commits atomic (should not break tests)

**Commit format:**
```
[Type] Short description (≤72 chars)

Longer explanation if needed.

Closes #123
Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
```

**Types:** `feat`, `fix`, `chore`, `docs`, `test`, `refactor`

## Common Make Targets

```bash
make setup              # Install all dependencies
make dev                # Start development environment
make test               # Run all tests
make lint               # Lint all code
make build              # Build all binaries
make docker-build       # Build Docker images
make clean              # Clean artifacts
make health             # Check service health
make info               # Show project info
```

See `make help` for full list.

## Deployment Verification

After deploying changes:

```bash
# Check pod status
kubectl --context <context> get pods -n nest

# View logs
kubectl --context <context> logs -n nest -l app=nest-api --tail=50

# Test API health
curl http://nest-api.nest.svc:8080/api/v1/health

# Verify metrics
curl http://nest-api.nest.svc:9090/metrics
```

## Troubleshooting

### "Cannot connect to Rook-Ceph"
- Verify Ceph is deployed: `kubectl get pods -n rook-ceph`
- Check StorageClass exists: `kubectl get sc`

### Tests fail locally but pass in CI
- Run with `-race` flag: `go test -race ./...`
- Check environment variables match CI
- Verify test database is clean

### Kubernetes deployment fails
- Check image is pushed: `docker image ls | grep nest`
- Verify context: `kubectl config current-context`
- Review pod events: `kubectl describe pod -n nest`

### Python API won't start
- Check dependencies: `pip list`
- Verify database connection: `echo $DATABASE_URL`
- Check Alembic migrations ran: `alembic current`

## Getting Help

- **Issues:** Check existing GitHub issues or create a new one
- **Docs:** See `/docs/` for comprehensive guides
- **Code review:** Ask in PR comments for clarification
- **Architecture:** See `/docs/standards/ARCHITECTURE.md`

## License

By contributing, you agree to license your work under the project's license. See [LICENSE.md](../LICENSE.md).
