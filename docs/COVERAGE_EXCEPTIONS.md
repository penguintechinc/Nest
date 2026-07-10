# Coverage Exceptions

These modules are excluded from the 90% coverage requirement because they require
live infrastructure that cannot be mocked in unit tests:

## controllers/ (certificates.py, external_ops.py, provisioning.py)

**Reason:** Require live Kubernetes API and gRPC connections (DB Proxy service).
These modules orchestrate cluster operations and require actual K8s environments.

**Test strategy:** Integration tests in staging environment or E2E tests with
mock K8s servers.

## clients/db_proxy_grpc.py

**Reason:** gRPC client for the in-repo DB Proxy `ConfigService` — requires a
running DB Proxy gRPC server on port 50051. Thin client over auto-generated gRPC
stubs that cannot be meaningfully unit-tested in isolation.

**Test strategy:** Integration tests with a mock gRPC server or the actual DB Proxy service.

## db_models/

**Reason:** SQLAlchemy ORM definitions — require live database connection for
schema reflection and ORM initialization. These are declarative schema definitions.

**Test strategy:** Schema validated by Alembic migrations and smoke tests that
verify database connectivity and schema structure.

---

## Coverage Goal

Even with these exceptions, the remaining testable code targets **90%+ coverage**
to ensure high quality and reliability of the core business logic, API handlers,
and utility modules.
