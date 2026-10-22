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

## Slice 2: Campaign domain, seed data and segment engine

Approved scope: versioned domain migration, campaign drafts, deterministic 5000-user seed, strict segment DSL and consent-aware audience preview. No LLM adapter, messages, approval or delivery execution yet.

- Schema: `migrations/002_campaign_domain.sql`; users, consents, carts, orders, draft campaigns and seed marker.
- Domain/queries: `internal/campaign`, `internal/segment`; validated types compile fixed SQL with bound values and mandatory channel consent.
- Storage: `internal/store/campaign.go`, `audience.go`; draft persistence and one repeatable-read transaction per aggregate preview.
- API: create/get drafts, PUT validated segment, POST audience preview. See README for the API contract and domain skill for DSL rules.
- Seed: `seed/seed.go`, enabled in Compose; one transaction and no overwrite on restart. Only non-dialable fake contact identifiers.
- Verification: unit/boundary tests, migration upgrade/repeat/rollback, PostgreSQL domain/seed tests, live API smoke and restart checks. Results belong in README.

## Slice 2.1: Segment persistence repair

Approved scope: preserve the 32768-byte HTTP ingress cap; revalidate canonical and PostgreSQL JSONB documents with a separate 262144-byte cap and identical DSL checks. Cover HTML escaping and JSONB whitespace expansion with unit, PostgreSQL and Postman round-trip regressions, including repeat save, preview and oversized HTTP rejection. No new endpoints, dependencies or migrations. Verification results belong in README.

## Slice 3A: Mock brief-to-segment

Approved scope: fixture-backed replaceable LLM provider, versioned segment prompt/schema, existing strict DSL validation, at most two repairs, bounded transient retry and timeout, metadata-only call audit (migration 003), and bodyless POST `/campaigns/{id}/generate-segment`. Persist only valid results on drafts, with state rechecked after generation. Include Go/PostgreSQL and maintained Postman tests. No SDK, real API calls, messages, approval, streaming or delivery implementation. See ADR-018 and README for behavior and verification evidence.

## Later slices

1. Campaign messages/edit/approval, real Claude integration and streamed messages.
3. PostgreSQL deliveries queue (ADR-006), worker, retries, idempotency and fault-injecting mock provider.
4. Vue interface, dashboard and metrics; final verification and sanitized session exports.

Each slice needs its own approved file plan. Final plain `docker compose up` must include seed and mock provider; tests require no real API key. Bonus scope remains deferred. No commits without explicit approval.
