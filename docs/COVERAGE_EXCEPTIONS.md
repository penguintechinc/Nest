# Coverage Exceptions

These modules are excluded from the 90% coverage requirement because they require
live infrastructure that cannot be mocked in unit tests:

## controllers/ (certificates.py, external_ops.py, provisioning.py)

**Reason:** Require live Kubernetes API and gRPC connections (dblb service).
These modules orchestrate cluster operations and require actual K8s environments.

**Test strategy:** Integration tests in staging environment or E2E tests with
mock K8s servers.

## clients/dblb_grpc.py

**Reason:** gRPC client stub — requires a running dblb gRPC server on port 50051.
Auto-generated gRPC code that cannot be meaningfully unit-tested in isolation.

**Test strategy:** Integration tests with mock gRPC server or actual dblb service.

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
