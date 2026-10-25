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

## Slice 3B: Mock message generation and streaming

Approved scope: selected-channel SMS/email validator with worst-case placeholder rendering, versioned message prompt/schema, mock chunks with configurable delay and failure scenarios, shared bounded call runner/auditing, and bodyless POST `/campaigns/{id}/generate-messages` over SSE. Events are attempt, provisional delta, then terminal validated/error. Recompute an aggregate consent-aware preview; keep messages ephemeral and campaign state draft. Migration 004 extends audit call names. Cover real incremental flushing, disconnects, repairs, limit boundaries and local Postman SSE assertions. No message persistence/editing/approval, frontend, real Claude or delivery. See ADR-019 and README for behavior and actual checks.

## Slice 3C: Real Claude adapter

Approved scope: official SDK v1.82.0 behind both validated provider interfaces, forced tools/schema/token bounds, incremental tool JSON decoding, strict terminal/tool checks, safe errors/usage metadata, explicit model/key configuration, test construction guard, mock default, and CA certificates in scratch. Preserve existing repair/retry/timeouts and public endpoint/SSE contracts. Verify SDK requests against local HTTP fixtures, plus decoder edge cases, full Go/PostgreSQL/Docker and mock-only Postman regressions. No real API verification, message persistence/approval, frontend or delivery changes. See ADR-020 and README.

## Slice 3D: Explicit message persistence and human editing

Approved scope: keep SSE generation preview-only; explicit full-set GET/PUT messages; reuse current audience bounds and domain validators; campaign-wide monotonic revisions with required If-Match on every mutation; retain text but invalidate readiness after segment writes; no content history or unverifiable provenance.

Implementation increments: (1) separate forced-tool model configuration correction; (2) migration 005, revision-aware domain/store/HTTP and existing Postman contracts; (3) migration 006, transactional message persistence/editing, coherent GenerationInput and base_revision SSE payload; (4) concurrency, cancellation, upgrade, rollback and local Postman lifecycle verification. Each increment is checked before advancing. Provider interfaces, mock default and existing validation/security boundaries remain. No approval, enqueue, worker, delivery, frontend or live Anthropic calls. See ADR-021/022 and README for contracts and evidence.

## Campaign approval and queue — first increment

Approved scope: implement the approval and PostgreSQL queue portions of the existing milestones together, without worker execution. See ADR-023 and README for canonical contracts and verification.

- Migration 007: approval source revision/fingerprint/preview, private frozen deliveries, database uniqueness and pending/due/lease fields.
- Domain/store: literal allowlisted rendering, current consent-aware validation, revision-locked atomic approval/enqueue, reviewed snapshot comparison and repeat reconciliation.
- HTTP/Postman: enriched audience-preview, conditional POST approve, aggregate GET delivery-status, strict errors and immutable approved content.
- Checks: unit/HTTP, Docker PostgreSQL races, same-count audience changes, multi-batch rollback, cancellation and 5000-user seed; local mock/test Postman passed 97 requests and 232 assertions. Full evidence is in README.
- No mock provider, worker, rate limiter, retry/recovery execution, frontend, real API calls or commits. Queue lifecycle columns prepare the next original milestone; they do not prove worker guarantees.

## Submission status

The original core milestones are implemented: backend/domain, Claude adapter with mock verification, explicit editing/approval, PostgreSQL worker/mock delivery, Vue interface, dashboard and metrics. Final review reconciles decision statuses, documents requirement coverage and exports authentic sanitized Codex conversations. See [docs/SUBMISSION_REVIEW.md](docs/SUBMISSION_REVIEW.md) for executed checks and the release checklist. This is submission work, not a new feature slice. Historical scope descriptions above describe each increment when it was approved.

Live Claude smoke verification remains unperformed and requires explicit authorization. Scheduling, A/B variants, per-user language and CI remain optional/deferred.

Each slice needs its own approved file plan. Final plain `docker compose up` must include seed and mock provider; tests require no real API key. Bonus scope remains deferred. No commits without explicit approval.

## Vue frontend, campaign dashboard and metrics

Approved on 2026-10-22 after reviewing the case study and existing API contracts. Implemented increments: (1) Vue/strict TypeScript/Vite list/create, cursor-paginated GET campaigns, tests and Compose web service; (2) readable audience criteria, cancellable POST SSE, explicit full-set editing/saves and revision/fingerprint review; (3) approval, polling delivery dashboard and aggregate Prometheus metrics from existing tables; (4) frontend/component/browser, Go/PostgreSQL and local Postman verification plus documentation. See ADR-025 and README for final behavior, evidence and remaining submission work.

Changed scope: web source/tests/tooling and Docker proxy; additive campaign list/metrics domain-store-HTTP files and tests; maintained Postman collection; Compose/Make/safety-scanner integration; README, decisions, plan and session index. No migration, real service call, queue redesign, protected-file access or commit.

## PostgreSQL queue worker and mock delivery

Approved continuation of the original deliveries milestone, not a new numbered slice. Implemented incrementally: queue claim/fenced acknowledgement/recovery transactions; durable deterministic mock HTTP provider; context-bounded worker/client with final consent check and shared channel admission; Compose wiring; unit/PostgreSQL/Postman verification. Migration 008 adds the mock acceptance/attempt/rate-window tables, channel admission state, expired-lease index and five-attempt constraint. API status names and campaign revisions remain unchanged. See ADR-024 and README for transaction boundaries, failure semantics and verification. The later Vue milestone implemented the dashboard and metrics through polling (ADR-025); final export/review status is recorded above.

## PostgreSQL 18.6 development upgrade

Approved on 2026-10-25: update the official digest-pinned Alpine image and PostgreSQL 18 volume layout, using fresh development storage. Retain application migrations and pgx; no historical data conversion. Update README and ADR-026, then verify make check, PostgreSQL integration, startup/restart persistence and local mock-only Postman behavior. Completed: checks, PostgreSQL integration, local mock-only Postman and container recreation persistence passed; see README for evidence.

## Focused campaign UX and safe draft deletion

Approved on 2026-10-25: make the empty-state plus focus the existing create form; replace repeated review checkboxes with a fresh audience/channel/saved-template confirmation dialog; preserve approval preconditions; add confirmed logical deletion for unapproved draft/content_ready campaigns with no approval or delivery history. Scope includes migration 009, guarded store/HTTP paths, Vue dialogs and state, Postman and regression tests, plus canonical documentation and session index. No application redesign, dependencies, real service calls or commit. See ADR-027 and README for contracts, executed verification and retention limitations.

## English/Turkish interface and OpenAPI documentation

Approved on 2026-10-25: typed lightweight UI dictionaries, visible switcher and Intl formatting, with authored campaign/AI data unchanged; canonical OpenAPI 3.2.1 and locally bundled Swagger UI; documentation/route/DTO/Postman consistency and locale/content regression tests. Scope includes Vue source/tests, pinned Swagger build dependency, a separate docs build entry/CSP, embedded spec endpoint and Docker source packaging, maintained Postman assertions and project/session documentation. Existing business API contracts, approval/deletion protections and database schema remain unchanged. No unrelated architecture, real service calls or commits. See ADR-028 and README for exact behavior, verification and limitations.
