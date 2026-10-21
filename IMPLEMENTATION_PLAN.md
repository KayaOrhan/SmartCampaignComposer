# Minimal end-to-end implementation plan

Status: Proposed; runtime implementation awaits approval. This is a file plan, not evidence of a working application.

## Approved documentation changes

- `AGENTS.md`: Codex tool-independent guardrails and explicit skill loading.
- `.claude/skills/ai-session-export/SKILL.md`: correct scrubber path.
- `.claude/skills/verify-before-done/SKILL.md`: metadata and documentation-only verification scope.
- `.claude/skills/campaign-domain/SKILL.md`: both-variable rendering bounds.
- `.claude/skills/delivery-pipeline/SKILL.md`: final consent timing and strict admission.
- `.claude/skills/llm-contract/SKILL.md`, `CLAUDE.md`: local test services allowed, external network forbidden.
- `DECISIONS.md`: supersede ADR-008; add ADR-013 and ADR-014.
- `README.md`: implementation status and known gaps.
- `INDEX.md` -> `ai-sessions/INDEX.md`: canonical session index and current entry.
- `IMPLEMENTATION_PLAN.md`: this plan.

## 1. Boot and enforce configuration

Create `go.mod`, `go.sum`, `Makefile`, `Dockerfile`, `compose.yaml`, `.dockerignore`, `scripts/lint.ps1`, `scripts/lint_test.ps1`, `internal/config/config.go`, `internal/config/config_test.go`, `cmd/api/main.go`, `cmd/worker/main.go`, `cmd/mockprovider/main.go`.

Inspect installed toolchains first. Verify dependency registry existence, maintainer and pinned versions before adding dependencies. Start with Go stdlib and a verified pinned Postgres driver; do not add the Anthropic SDK until the real adapter is approved. `APP_ENV=test` forbids real LLM construction. Strict config validation allowlists the mock provider origin and rejects redirects to other origins. Never add a real messaging SDK.

Make targets: `up` starts Compose and seed; `test` runs isolated tests with local Postgres and mock HTTP only, without external network; `lint` checks formatting, forbidden provider imports, secrets and invisible/bidirectional Unicode; `check` requires both lint and tests. A failed check must propagate nonzero. Secret scanning must redact findings and exclude protected local secret files without reading them; explicit tracked/staged path checks reject them. Verify scanners using synthetic fixtures, never real secrets. Docker build context excludes secrets and raw exports.

## 2. Durable campaign and validated mock generation

Create `migrations/001_init.sql`, `seed/seed.go`, `seed/seed_test.go`, `internal/store/store.go`, `internal/store/store_test.go`, `internal/segment/segment.go`, `internal/segment/segment_test.go`, `internal/campaign/campaign.go`, `internal/campaign/campaign_test.go`, `internal/llm/llm.go`, `internal/llm/llm_test.go`, `internal/llm/prompts/extract-v1.txt`, `internal/llm/prompts/messages-v1.txt` and one JSON fixture per scenario named in the LLM skill under `internal/llm/testdata/`.

Use deterministic fake data. Implement strict JSON and semantic validation, field/operator allowlists, bound SQL parameters, channel consent filters, both-variable rune bounds and all message limits. Implement the mock LLM interface, bounded repair loop, typed errors, call records without recipient data and SSE final validation gate. Do not construct a real API client in the mock slice. Add injection, malformed-output, repair, cancellation and timeout tests alongside code.

## 3. Approve, deliver, observe

Create `internal/delivery/delivery.go`, `internal/delivery/delivery_test.go`, `internal/delivery/integration_test.go`, `internal/delivery/mockprovider.go`, `internal/delivery/mockprovider_test.go`, `internal/campaign/http.go`, `internal/campaign/http_test.go`; wire existing entrypoints.

Approval validates and freezes payloads in one transaction with enqueueing and database uniqueness. Define legal state transitions, repeat approval behavior, cancellation and zero-recipient rejection explicitly in tests. Implement SKIP LOCKED claims, deterministic provider idempotency, durable mock deduplication across restart, paced admission, final consent/cancellation gate, bounded retries and stale-claim recovery. Fence completion updates against stale workers. Never send outside the allowlisted mock. Propagate context deadlines through database and HTTP calls; log structured IDs without recipient data.

Verify concurrent workers and crash recovery, zero duplicate accepted messages, rate-limit windows, consent revoked while waiting, consent query errors, 429 Retry-After, permanent 4xx, attempt exhaustion and stale worker updates. Expose health, readiness, metrics and database-backed progress SSE. Local HTTP and disposable Postgres are permitted; external network and real API keys are not required.

## 4. Minimal Vue flow and completion evidence

Create `web/package.json`, `web/package-lock.json`, `web/index.html`, `web/tsconfig.json`, `web/vite.config.ts`, `web/src/main.ts`, `web/src/App.vue`, `web/src/App.test.ts`, `web/src/style.css`. Verify and pin frontend dependencies before installation. Configure test tooling in the existing Vite config.

Build brief -> audience preview -> generated messages -> human edit/revalidation -> approve -> delivery status. Partial SSE content is display-only; approval requires backend validation and backend state enforcement. Exercise the complete mock flow and reject approval before validation or after invalid edits. Update README with exact run commands and evidence. Mark ADRs Accepted only after their implementation and relevant tests pass. Run `make check`, inspect the actual diff and export/scrub the session before submission. No commit without explicit approval.

## Boundaries

The first slice uses the mock LLM and mock delivery only. Real Claude streaming gets a separate approved spike after the mock contracts work. Runtime Go/Vue checks cannot pass until their files exist; do not add placeholder targets that report success. Any necessary file-list expansion must be reported in the implementation plan before dependent edits.
