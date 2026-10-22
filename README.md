# Smart Campaign Composer

Slices 1, 2 and 3A implement the Go/PostgreSQL foundation, campaign drafts, fake seed data, validated audience previews and mock brief-to-segment generation. Requirements: [docs/CASE_STUDY.md](docs/CASE_STUDY.md). Architecture: [DECISIONS.md](DECISIONS.md). Safety: [RULES.md](RULES.md).

## Run

Install Docker with Compose and start its Linux container engine. From the repository root:

```sh
docker compose up
```

Compose builds a missing API image, waits for PostgreSQL health, and starts the API on `127.0.0.1:8080`. PostgreSQL is not exposed to the host. The `app` credentials are disposable local development defaults. Data persists in a named volume. After source changes, use `docker compose up --build` to rebuild an existing image.

```sh
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
docker compose down
```

`/health` returns JSON 200 without querying PostgreSQL. `/ready` returns JSON 200 after a database ping, or 503 on failure/timeout without exposing database details. These are the canonical endpoints (ADR-015).

Configuration uses environment variables only: `DATABASE_URL` (required), `HTTP_ADDR` (default `:8080`), `DB_TIMEOUT` (default `2s`, maximum `30s`), and `SEED_DATA` (default `false`, accepts only `true`/`false`). Names appear in `.env.example`. Compose enables seeding: startup transactionally inserts 5000 fake users, per-channel consents, carts and orders once. Emails use `@example.test`; phone values are non-dialable identifiers. Subsequent startup preserves data and consent edits. The recorded seed reference time stays fixed, so development events age naturally.

No dotenv loader is used. Later Claude integration must accept `ANTHROPIC_API_KEY` supplied by Compose from user configuration; agents must never inspect that file.

## Campaign API

| Method and path | Request | Success |
| --- | --- | --- |
| `POST /campaigns` | JSON `brief` and `channels` | 201 draft plus Location header |
| `GET /campaigns/{id}` | No body | 200 stored campaign |
| `PUT /campaigns/{id}/segment` | Segment DSL JSON | 200 updated draft |
| `POST /campaigns/{id}/generate-segment` | No body | 200 updated draft with validated mock segment |
| `POST /campaigns/{id}/audience-preview` | No body | 200 aggregate preview |

Briefs must contain 1 to 4000 runes after trimming. Channels are a nonempty unique selection of `sms`/`email`, stored in canonical order. Segments can only be changed on drafts. There is no message generation, approval or send endpoint yet. Preview without a segment returns 409.

Example segment:

```json
{"version":1,"all":[{"field":"city","op":"in","value":["istanbul"]},{"field":"cart_added","op":"within_days","value":30},{"field":"purchase","op":"none_within_days","value":30}]}
```

The [campaign-domain skill](.claude/skills/campaign-domain/SKILL.md) specifies allowed fields, operators, limits, normalization and UTC window boundaries. Consent is always added by the compiler and cannot be supplied or removed by a segment. All values become bound SQL parameters; segment text is never executed.

Preview returns `as_of`, `eligible_unique` (the union across selected channels), and `by_channel` with `count`, `max_first_name_runes` and `max_city_runes`. Both maxima are independent, even when different recipients supply them. No recipient rows or contact details are returned. A repeatable-read transaction keeps channel counts consistent; this is not an approval snapshot.

Requests containing JSON require Content-Type `application/json`. Validation rejects duplicate or unknown keys, wrong types, invalid UTF-8 and trailing values. Error responses have the form `{"error":{"code":"unsupported_field","path":"all[0].field"}}`, without echoing submitted values or driver errors. Statuses: 400 malformed JSON, 413 oversized body, 422 invalid contract, 404 missing campaign, 409 invalid state/missing segment, 503 database unavailable.

## Tests and checks

Install Go 1.27 and GNU Make; Docker is needed for integration tests.

```sh
make test
make check
make test-integration
```

Unit tests require no database, external API or API key. Initial downloads of pinned modules require registry access. `make check` runs safety/format checks, `go vet` and unit tests. The scanner rejects tracked secret paths without reading their contents and checks safe source files for known key patterns and hidden Unicode; matching values are never printed.

Integration tests create the dedicated disposable `scc_test` database if needed and reset only its test schema/tables. Do not store valuable data there. They verify migration upgrade/repeat/rollback, consent-aware matching, event boundaries, duplicate-event handling, draft restrictions, injection-safe queries and seed repeat/rollback. The test image is rebuilt to include current source. No local filesystem is mounted into that container.

Safety evidence for this slice:

| RULES.md requirement | Evidence |
| --- | --- |
| Rule 1: model output never executes as SQL | `TestLLMOutputNeverReachesSQL`; `TestDomain` also runs the injection value against PostgreSQL and verifies user rows survive |
| Rule 3: channel consent required | `TestDomain`: missing/denied consent, revocation, channel overlap and single-channel selection; dispatch rechecks belong to the delivery slice |
| Rules 5/7: no secret disclosure or secret-file reads | `TestProtectedPaths`, `TestInspect` and HTTP error-redaction cases; agent verification uses the safe template instead of dotenv |
| Rule 8: verify/pin dependencies | Existing x/text version promoted without a version change; registry origin recorded in ADR-016; `go mod verify` passed |
| Bounded calls and UTC | `TestCampaignContextCancellation`, `TestQueryWindowsAndZeroValue`, cancelled repository queries and PostgreSQL boundary cases |
| Repeat-safe operations | `TestMigrations`, `TestSeedPersistence`, `TestSeedFailureRollsBack` and repeated segment-update integration checks |

Without Make, equivalent local checks are:

```sh
go run ./scripts
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

For agent-run Compose checks use `docker compose --env-file .env.example ...` or set `COMPOSE_DISABLE_ENV_FILE=1` to avoid automatic reads of user secrets. The user launch command remains plain `docker compose up`.

## Migrations

Numbered `migrations/*.sql` files are embedded at build time. One startup transaction creates the ledger, applies pending files in filename order and records their names. Applied files must not be edited; use ascending, zero-padded prefixes for new files. Failure rolls back and prevents API startup. Only one API instance is supported; coordination and down migrations are intentionally absent.

## Verification status

Executed successfully on 2026-10-21: Go unit tests, vet, safety/format checks, module checksum verification, Docker image builds, Compose configuration validation and plain `docker compose up` (agent disabled automatic dotenv reads). PostgreSQL integration tests passed, covering apply/repeat/rollback. HTTP probes observed `/health=200`, `/ready=200`; with PostgreSQL stopped, `/health=200` and `/ready=503`; readiness recovered to 200 after restart. A deliberate liveness-status mutation made the endpoint test fail, and the source was restored.

Slice 2 checks executed on 2026-10-21: `make check` passed (safety/formatting, vet and all unit tests); `make test-integration` passed against local PostgreSQL. Docker rebuilt and upgraded the existing database; aggregate SQL confirmed `5000` users, one seed marker and two migration records. HTTP smoke checks observed create=201, segment save=200, preview=200 and invalid-field rejection=422 with the prior stored segment preserved. The sample preview returned 727 distinct eligible recipients (SMS 467, email 571); these are illustrative, time-dependent counts. A deliberate consent-filter mutation made `TestLLMOutputNeverReachesSQL` fail; the original source was restored and the test passed.

Make is now available. Race testing remains blocked by missing CGO/compiler support. The bundled skill validator passed (`Skill is valid!`) using verified, pinned PyYAML 6.0.3 installed only in a temporary tooling directory; no Python dependency was added to the project. The skill's description limits were also checked separately. Full assignment functionality is not implemented or verified.

## Known gaps

- Real Claude, streaming, campaign messages/approval, mock delivery provider, worker, metrics and Vue are not implemented yet. The mock segment generator always returns the sample Istanbul/cart/no-purchase fixture and does not interpret arbitrary briefs. Final Compose startup must also include the mock provider.
- PostgreSQL remains the queue choice (ADR-006).
- Local race testing lacks a CGO compiler.
- Secret scanning detects common formats, not all possible secrets. Claude hooks and session scrubbing remain unverified.
- Current `.gitignore` excludes all `ai-sessions/`; narrow this to raw exports before preparing sanitized submission records.
- Later domain limitations remain in ADR-002/005/013/014: abandonment approximation, SMS segment estimates, final consent race, per-channel ownership and conservative rendering bounds.
- Scheduling, A/B variants, per-user language and CI are optional and deferred.

## Slice 2.1 verification (2026-10-22)

Segment HTTP input remains capped at 32768 bytes. Canonical and stored JSONB representations use the separate bounded parser described in ADR-017; all DSL validation remains shared. No endpoint, dependency or migration was added.

Executed successfully: `make check`; `make test-integration` against local Docker PostgreSQL; `docker compose --env-file .env.example up -d --build api`; and the documented local Postman collection command. Postman executed 31 requests and 82 assertions, zero failures, exit 0. Generated regressions cover HTML escaping, JSONB whitespace expansion, read/preview/repeat-save and oversized HTTP rejection preserving the saved document. The PostgreSQL test asserts that its compact Unicode fixture crosses the ingress byte limit specifically after JSONB storage.

Actual check output included:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

All listed Go packages passed. Temporarily changing the document parser back to the ingress cap caused `TestDocumentRoundTrip` to fail with `canonical round trip failed body_too_large at $`; source was restored before the final checks. The changed domain skill passed the bundled validator (`Skill is valid!`) using the existing temporary PyYAML tooling installation.

Safety coverage: Rule 1 and boundary validation are exercised by `TestValidation`, `TestDocumentRoundTrip` and `TestLLMOutputNeverReachesSQL`; Rule 3 remains covered by `TestDomain`; repeat-safe persistence is covered by `TestSegmentPersistenceRoundTrip`. Rules 5/7 are supported by the repository safety check and explicit Compose template use. Rule 9 is supported by the deliberate failing regression control. No external LLM/provider calls or new dependencies were introduced. Race testing remains blocked by the existing missing CGO compiler. The local collection creates one persistent synthetic draft per full run.

## Mock brief-to-segment (3A)

POST `/campaigns/{id}/generate-segment` with no body uses the stored draft brief and replaces its segment only after successful validation and call recording. The response is the same campaign representation as GET. Repeat calls keep the same deterministic segment; each invocation creates fresh call records. Concurrent draft edits use last completed save; draft state is rechecked under lock at save time. There is no database lock held during generation. Preview remains a separate request.

`LLM_PROVIDER` defaults to `mock`; other providers fail startup, including `anthropic` under `APP_ENV=test`. No API key is needed or read by this implementation. `LLM_TIMEOUT` defaults to `10s` and must be positive and at most `30s`; it bounds the entire generation sequence. `LLM_MOCK_SCENARIO` defaults to `happy`; supported segment scenarios are `happy`, `unknown_field`, `extra_keys`, `no_tool_use`, `refusal`, `timeout`, `rate_limited`, `repair_succeeds` and `repair_exhausted`. These are startup/test controls, never request fields. Compose selects `happy`. The mock returns fixtures independently of brief wording; it is a test adapter, not a language parser.

The provider interface receives the versioned prompt, forced tool name, schema, token bound and safe validation feedback. Every tool input goes through the existing strict DSL validator. There are at most two validation repairs and one transport retry per attempt for rate-limit/transient failures, with bounded backoff. No model output is executed. Failed output or refusal returns 502; timeout/cancellation returns 504; unavailable provider or audit storage returns 503. Validation exhaustion adds `error.reasons`, an array of safe code/path pairs. No raw model/driver error is returned. Failures preserve the previous segment.

Migration 003 adds `app.llm_calls`: campaign ID, call name, prompt version, outcome, safe validation errors, latency, token counts and UTC creation time. Each provider attempt has a record, including transport retries and timeout failures. Mock token counts are zero, not estimated production usage. No briefs, prompts, raw outputs or recipient records are stored in this table. A failed audit insert prevents segment persistence. Cancellation audit writes have a separate bounded two-second cleanup budget; if storage is unavailable, a durable record cannot be guaranteed and the request fails closed.

### 3A verification (2026-10-22)

Executed `make check` (safety/formatting, vet and unit tests), `make test-integration` (local disposable PostgreSQL), Compose API rebuild and the documented local Postman collection run. PostgreSQL tests cover migration upgrade/repeat/rollback, happy and repaired generation persistence, exhausted repairs preserving the prior segment, per-attempt audit counts, cancellation audit and audit outage. Postman passed 40 requests and 104 assertions, zero failures, exit 0, including generation/read/preview/repeat and rejection of scenario bodies, bad IDs, missing campaigns and oversized bodies. Reporting stayed local.

Actual check output:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

The listed Go packages passed. A deliberate temporary validation bypass made `TestSegmentValidationAndPromptBoundary` fail with `invalid output did not fail closed <nil>`; the original code was restored. Rule 1/boundary validation is covered by `TestSegmentValidationAndPromptBoundary` and `TestMockSegmentScenarios`; Rule 3 by the existing `TestDomain`; Rule 6 by `TestRealLLMForbiddenInTest` and the network-free mock; Rule 9 by the deliberate failure control. Rules 5/7 are supported by metadata-only audit types, error-redaction tests, the repository safety check and explicit Compose template use. No dependency, real LLM request, delivery or commit was introduced. Race testing remains blocked by missing local CGO/compiler support.
