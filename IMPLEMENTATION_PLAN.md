# Implementation slices

The authoritative product assignment is [docs/CASE_STUDY.md](docs/CASE_STUDY.md). Safety requirements remain in [RULES.md](RULES.md).

## Slice 1: Backend foundation

Approved scope: Go module, standard-library HTTP API, PostgreSQL pool with context deadlines, liveness/readiness, numbered SQL migrations, Docker Compose, Make targets and tests.

- API: `cmd/api`, `internal/httpapi`; GET `/health` and `/ready`.
- Database: `internal/store`, `migrations`; trusted embedded SQL applied transactionally at startup and recorded by filename. One API instance; no checksum framework or rollback CLI.
- Runtime: `Dockerfile`, `.dockerignore`, `docker-compose.yml`; API and PostgreSQL by default. Optional test profile uses disposable `scc_test`, without mounting local files.
- Checks: `make test`, `make lint`, `make check`, `make test-integration`.
- Launch: plain `docker compose up` builds missing images and starts services. Seed data and the mock provider belong to later slices.

Verification evidence and limitations belong in [README.md](README.md). Do not claim a configured command has passed without actual execution.

## Later slices

1. Validated segment DSL, consent-aware fake data and mock LLM contracts.
2. Campaign preview/edit/approval, Claude integration and streamed messages.
3. PostgreSQL deliveries queue (ADR-006), worker, retries, idempotency and fault-injecting mock provider.
4. Vue interface, dashboard and metrics; final verification and sanitized session exports.

Each slice needs its own approved file plan. Final plain `docker compose up` must include seed and mock provider; tests require no real API key. Bonus scope remains deferred. No commits without explicit approval.
