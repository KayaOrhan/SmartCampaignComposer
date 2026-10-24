# Smart Campaign Composer

Vue 3 + TypeScript campaign management, validated AI audience/message suggestions, explicit human editing and approval, and a PostgreSQL queue with mock delivery. The dashboard shows delivery progress and aggregate metrics. Requirements: [docs/CASE_STUDY.md](docs/CASE_STUDY.md). Architecture: [DECISIONS.md](DECISIONS.md). Safety: [RULES.md](RULES.md).

## Technology stack and pinned versions

Versions below describe repository pins, not versions detected on the host. Container digests are recorded in the linked build/configuration files.

| Component | Version / repository source |
| --- | --- |
| Go API, worker and mock provider (`net/http`, `database/sql`) | Module language version `1.27.0` in [go.mod](go.mod); builder `golang:1.27.3-alpine`, digest-pinned in [Dockerfile](Dockerfile); runtime `scratch` |
| PostgreSQL relational store and queue | `postgres:18.6-alpine@sha256:898c60cb390d1085bc97bdd6a1ae968757421df394b071848b52b1c09c89d6a5` in [docker-compose.yml](docker-compose.yml) |
| Go direct dependencies | `pgx/v5 v5.11.0`, `anthropic-sdk-go v1.82.0`, `golang.org/x/text v0.29.0` in [go.mod](go.mod); transitive checksums in [go.sum](go.sum) |
| Frontend | Vue `3.5.44`, TypeScript `5.9.3`, Vite `8.3.5`, Vue Vite plugin `6.0.10`, vue-tsc `3.3.13` in [web/package.json](web/package.json) |
| Frontend tests | Vitest `5.0.5`, Vue Test Utils `2.5.1`, happy-dom `21.0.4`, Playwright `1.64.1`; exact dependency graph in [web/package-lock.json](web/package-lock.json) |
| Frontend build / HTTP server | `node:22-alpine` and `nginx:stable-alpine`, both digest-pinned in [web/Dockerfile](web/Dockerfile). Tags do not specify exact patch versions; the digests fix image identity. |
| Orchestration | Docker Compose; no Docker engine or Compose CLI version is pinned in the repository |
| LLM | Mock fixtures by default; optional Claude via the pinned official Go SDK. Compose's model default is `claude-sonnet-4-6`, a model identifier rather than a verified live-service version. |

PostgreSQL serves data and queue with fewer services (ADR-001/006); official container images are pinned by digest (ADR-015). ADR-026 records the PostgreSQL 18.6 development upgrade and fresh-volume policy.

## Run

Install Docker with Compose and start its Linux container engine. From the repository root:

```sh
docker compose up
```

Compose builds missing images, waits for PostgreSQL health, and starts the frontend on `127.0.0.1:5173`, API on `127.0.0.1:8080`, mock provider on `127.0.0.1:8081` and a separate worker. PostgreSQL is published on `127.0.0.1:5433`, mapped to container port `5432`; backend containers connect to `db:5432`. The `app` credentials are disposable local development defaults. Data persists in a named volume. After source changes, use `docker compose up --build` to rebuild existing images. The API owns migrations; worker/provider fail closed and restart if their schema is not ready during startup.

```sh
curl -i http://localhost:8080/health
curl -i http://localhost:8080/ready
docker compose down
```

`/health` returns JSON 200 without querying PostgreSQL. `/ready` returns JSON 200 after a database ping, or 503 on failure/timeout without exposing database details. These are the canonical endpoints (ADR-015).

Configuration uses environment variables only: `DATABASE_URL` (required), `HTTP_ADDR` (default `:8080`), `DB_TIMEOUT` (default `2s`, maximum `30s`), and `SEED_DATA` (default `false`, accepts only `true`/`false`). Names appear in `.env.example`. Compose enables seeding: startup transactionally inserts 5000 fake users, per-channel consents, carts and orders once. Emails use `@example.test`; phone values are non-dialable identifiers. Subsequent startup preserves data and consent edits. The recorded seed reference time stays fixed, so development events age naturally.

No dotenv loader is used. Compose passes explicitly configured `ANTHROPIC_API_KEY` and `ANTHROPIC_MODEL` to the API in Anthropic mode; agents never inspect `.env`. Mock mode remains the default.

No environment file or API key is needed for a fresh mock demonstration. To avoid automatic loading of a local dotenv file, use `docker compose --env-file .env.example up --build`. Keep `LLM_PROVIDER=mock`; inherited shell variables override the template. Automated review explicitly selected `APP_ENV=test` and cleared the API key. For real Claude configuration, the operator supplies their own `ANTHROPIC_API_KEY` through Compose environment interpolation (a local ignored dotenv file or exported variable), selects `LLM_PROVIDER=anthropic` and uses a non-test `APP_ENV`. Automated tests never exercise that mode.

The template lists process configuration; it does not make every variable a Compose override. Compose fixes `DATABASE_URL`, addresses, `DB_TIMEOUT`, `SEED_DATA=true`, mock scenarios/chunk delay, worker timing and `FAULT_SEED` in its service definitions. Only `${...}` entries there use shell/template interpolation (LLM mode/model/key/timeout, `APP_ENV`, worker concurrency and fault rates/latency). For a Go process launched on the host, use `127.0.0.1:5433` in `DATABASE_URL`; the template's `db:5432` hostname is internal to Compose. Host-run worker calls use the allowlisted loopback mock endpoint on port 8081.

## Architecture and demo

The Vue SPA calls the Go API through a same-origin nginx `/api` proxy. The API validates Claude tool output, compiles the segment DSL with bound SQL parameters, and stores campaigns in PostgreSQL. Human saves and approval use revision tokens; approval freezes personalized payloads and enqueues them atomically. A separate Go worker consumes the PostgreSQL queue, rechecks consent, and calls only the mock provider. Shared database admission limits each channel; the durable mock ledger deduplicates retries. The dashboard polls persisted progress; metrics expose fixed-label aggregates.

### PostgreSQL topology and choice

The default Compose application has **one PostgreSQL instance** (`db`), **one application database** (`app`), and **one application schema** (`app`). API, worker and mockprovider all use the same database URL and `app` database role; they have separate connection pools, not separate databases. The named `postgres18_data` volume mounts `/var/lib/postgresql` and persists PostgreSQL 18's `/var/lib/postgresql/18/docker` data directory. This development upgrade starts fresh and applies all existing application migrations and seed data; older volumes are not reused.

| Stored state | Location in database `app` |
| --- | --- |
| Profiles, consents, shopping events, campaigns and templates | `app.users`, `app.consents`, `app.cart_events`, `app.orders`, `app.campaigns`, `app.campaign_messages` |
| Transactional delivery queue and channel admission | `app.deliveries`, `app.delivery_channels` |
| Mock provider acceptance deduplication, attempts and rate window | `app.mock_acceptances`, `app.mock_attempts`, `app.mock_calls` |
| LLM audit / seed marker | `app.llm_calls`, `app.seed_runs` |
| Migration ledger (schema exception) | `public.schema_migrations` |

[Migrations 001-008](migrations) define the application schema; [the migration runner](internal/store/migrate.go) creates the ledger separately in `public`. The optional test profile targets a **different database**, `scc_test`, on the same `db` instance. It is disposable test storage, not a separate production queue database.

PostgreSQL fits the relational model: foreign keys connect campaigns, users, consents and deliveries; constraints enforce valid states and unique campaign/user/channel jobs; JSONB stores validated segment documents alongside relational records. Reusing it as the queue lets approval, frozen payload insertion and campaign state commit or roll back together. There is no database-to-broker dual write in this implementation. ADR-001/006 describe the choice, and ADR-023/024 describe the implemented transaction and worker semantics.

The trade-off is shared capacity and failure: queue polling, updates, indexes, cleanup and admission locks compete with API queries; a database outage affects campaigns, workers and mock acceptance together. Queue scheduling, leases, recovery and retry policy are application code. The requirement is a maximum of 100 messages/second/channel, not a promised achieved throughput; response-plus-10ms serialization can be slower. No broker comparison benchmark establishes a universal performance advantage.

| Alternative | When it may be preferable | Cost relative to this demo |
| --- | --- | --- |
| RabbitMQ | A dedicated message broker with routing and consumer acknowledgement requirements, or independently operated queue capacity | Another service and operational lifecycle; atomic campaign approval plus publication would need a reliable handoff such as an outbox and relay, with consumer idempotency still required |
| Kafka | A retained event log with replay and multiple independent consumers, or a broader event-streaming architecture | Another distributed system and a different consumption model; campaign-to-log publication, delivery deduplication and delayed retries would still need design |

These are scope trade-offs, not claims that PostgreSQL is universally superior. RabbitMQ is listed among ADR-006's alternatives; Kafka is a comparison here, not a recorded historical evaluation or rejected implementation.

### API to queue to mock delivery

```text
Browser -> nginx /api -> Go API
                           |
                           v
              PostgreSQL app database
              app.campaigns + app.deliveries (atomic approval/enqueue)
                           |
                           v
                     Go worker -> HTTP mockprovider /messages
                           |                  |
                           v                  v
              app.deliveries status    app.mock_acceptances
                           |
              API aggregate reads -> browser polling / metrics
```

Approval validates the revision, reviewed audience fingerprint, saved content and consent-aware audience, then freezes personalized jobs in one transaction. The worker claims due rows with `FOR UPDATE SKIP LOCKED`, applies shared per-channel admission, rechecks consent and sends the frozen payload with a deterministic idempotency key. The mock records matching-key acceptance durably in the same database. The worker then records success, retry, dead or skipped status in a separate transaction; no database transaction spans HTTP. Leases recover abandoned work. At-least-once attempts and mock deduplication prevent duplicate simulated acceptance; acceptance and acknowledgement remain separate commits. The detailed failure semantics are in [PostgreSQL worker and mock delivery](#postgresql-worker-and-mock-delivery).

### Authentication, authorization and exposure

There is **no application authentication or authorization**: no login, session/token verification, roles, tenant isolation or campaign ownership checks. Anyone who can reach the API directly on port 8080 or through the web `/api` proxy can read campaigns/templates, create and edit campaigns, request generation, and approve mock delivery subject to the same validation/state rules. Health, metrics and mock debug/ingress endpoints are also unauthenticated. The worker has no published HTTP port.

`If-Match` revisions prevent stale writes; audience fingerprints detect changed review inputs; consent checks protect recipient eligibility. None identifies or authorizes a caller. The human-review checkbox is a UI safeguard, not server-enforced reviewer identity. `APP_ENV=production` does not add access control. All backend services share the powerful database role used for initialization/migrations; there is no per-service least-privilege role separation.

Current host bindings are loopback only, and Compose configures database traffic with `sslmode=disable`; HTTP has no configured TLS termination. Loopback limits network reach, not access by local processes. Publishing ports on public interfaces or forwarding the web UI through a tunnel exposes campaign content, unauthenticated mutations/approval, metrics and resource consumption. In optional Anthropic mode, unauthenticated generation can also spend the operator's API budget. Exposing PostgreSQL with the demo credentials risks direct data modification and bypasses application validation; exposing mock ingress permits fabricated simulated acceptances. Mock-only destination/transport restrictions prevent real SMS/email, but do not provide access control. Public deployment requires authenticated access and server-side authorization, restricted database/provider access, proper credentials and TLS; this repository does not implement those boundaries.

### Demo walkthrough

1. Open [Campaign Studio](http://127.0.0.1:5173), select SMS and email, and create a brief such as: "A short, friendly 30% offer for Istanbul users who added to cart but did not purchase in the last 30 days."
2. Generate the segment and preview audience counts. Mock mode returns the documented sample segment regardless of brief wording.
3. Generate messages and watch the provisional stream become a validated suggestion. Edit the SMS or email subject, then click **Save messages**.
4. Click **Review current audience**, inspect counts/content and check the human-review checkbox. Click **Approve & start mock delivery**.
5. Watch pending/in-flight jobs become sent/dead/skipped until completed. Return to the list and open **View metrics**. Completed means all jobs are terminal, not that every job succeeded.

The final requirement matrix, current test evidence and release checklist are in [docs/SUBMISSION_REVIEW.md](docs/SUBMISSION_REVIEW.md). Authentic sanitized development transcripts and provenance are in [ai-sessions/INDEX.md](ai-sessions/INDEX.md).

## Campaign API

Open [Campaign Studio](http://127.0.0.1:5173) for the complete demo: create a brief with SMS/email channels, generate a segment, preview consent-filtered counts, generate messages, edit and explicitly save, review the current audience, then approve mock delivery. The delivery panel shows pending/in-flight/sent/dead/skipped jobs and stops polling when the campaign completes. Completion can include dead/skipped jobs. Generated previews are plain text and never implicitly saved or approved.

For frontend development with the Compose API already running: `npm --prefix web ci --ignore-scripts`, then `npm --prefix web run dev`. Stop the Compose web service first if it occupies port 5173. Both Vite and nginx proxy /api to the Go API; nginx disables buffering for SSE. No browser secrets or dotenv files are required.

| Method and path | Request | Success |
| --- | --- | --- |
| `GET /campaigns` | Optional positive `before` ID and `limit` (default 20, maximum 100) | 200 `{campaigns: [...], next_before: number or null}`, descending ID |
| `GET /metrics` | No body | 200 Prometheus text aggregates; 503 if PostgreSQL is unavailable |
| `POST /campaigns` | JSON `brief` and `channels` | 201 draft plus Location header |
| `GET /campaigns/{id}` | No body | 200 stored campaign |
| `PUT /campaigns/{id}/segment` | Segment DSL JSON, required If-Match | 200 updated draft |
| `POST /campaigns/{id}/generate-segment` | No body, required If-Match | 200 updated draft with validated segment |
| `POST /campaigns/{id}/generate-messages` | No body | 200 SSE message preview; details below |
| `POST /campaigns/{id}/audience-preview` | No body | 200 aggregate preview |
| `GET /campaigns/{id}/messages` | No body | 200 saved templates or messages=null |
| `PUT /campaigns/{id}/messages` | JSON messages envelope, required If-Match | 200 saved, revalidated content_ready templates |
| `POST /campaigns/{id}/approve` | JSON approval_fingerprint, required If-Match | 200 approval receipt and persisted queue counts |
| `GET /campaigns/{id}/delivery-status` | No body | 200 PostgreSQL progress counts, no recipient details |

List query values must be canonical positive decimal integers. Unknown/duplicate parameters, invalid encodings and limits above 100 return 422 invalid_pagination. Pass next_before as the next before cursor; null means the last page. Concurrent inserts do not shift the remaining pages. The list has no recipient information and does not provide a campaign mutation ETag; fetch the chosen campaign for its revision.

GET /metrics returns Prometheus text format 0.0.4 with no-store caching. It reads one bounded, read-only repeatable-read database snapshot and exposes only fixed-label aggregates:

| Metric | Meaning |
| --- | --- |
| `deliveries{channel,status}` (gauge) | Current persisted jobs in each of pending, in_flight, sent, dead and skipped, including zero values |
| `queue_depth{channel}` (gauge) | Pending plus in-flight jobs, including retry waits |
| `llm_calls_total{call,outcome}` (counter) | Audited provider attempts, including retries/repairs; call is ExtractSegment or GenerateMessages |
| `llm_validation_failures_total{call}` (counter) | Number of recorded schema/domain validation errors, not failed campaigns or individual recipients |

Counters persist across process restarts and reset when the database is reset. Metrics include no campaign/user identifiers, arbitrary error text, destinations or message payloads. Proposed attempt-outcome/provider-latency series are not emitted because the existing records cannot establish them. The case study requires metrics but does not prescribe specific series. Health/readiness remain unchanged.

Briefs must contain 1 to 4000 runes after trimming. Channels are a nonempty unique selection of `sms`/`email`, stored in canonical order. Segment and content changes are allowed in draft/content_ready only. Preview without a segment returns 409. Approval/enqueueing and mock delivery execution are implemented.

Example segment:

```json
{"version":1,"all":[{"field":"city","op":"in","value":["istanbul"]},{"field":"cart_added","op":"within_days","value":30},{"field":"purchase","op":"none_within_days","value":30}]}
```

The [campaign-domain skill](.claude/skills/campaign-domain/SKILL.md) specifies allowed fields, operators, limits, normalization and UTC window boundaries. Consent is always added by the compiler and cannot be supplied or removed by a segment. All values become bound SQL parameters; segment text is never executed.

Preview returns `as_of`, `eligible_unique` (the union across selected channels), `by_channel` with `count`, `max_first_name_runes` and `max_city_runes`, plus `revision` and `approval_fingerprint`. Both maxima are independent, even when different recipients supply them. No recipient rows or contact details are returned. A repeatable-read transaction keeps channel counts consistent. For content_ready, it revalidates saved templates and computes an approval comparison fingerprint; drafts and empty audiences return a null fingerprint. See the approval contract below.

Requests containing JSON require Content-Type `application/json`. Validation rejects duplicate or unknown keys, wrong types, invalid UTF-8 and trailing values. Error responses have the form `{"error":{"code":"unsupported_field","path":"all[0].field"}}`, without echoing submitted values or driver errors. Statuses: 400 malformed JSON/revision token, 413 oversized body, 422 invalid contract, 404 missing campaign, 409 invalid state/missing segment/concurrent transaction, 412 stale revision, 428 missing revision precondition, 503 database unavailable.

## Saved messages and editing (3D)

Generate -> validate preview -> explicitly save -> retrieve/edit -> revalidate -> content_ready. A validated SSE event is neither persistence nor approval. Its base_revision identifies the coherent campaign/audience snapshot used for generation. Generation can run from draft or content_ready; it never changes stored text, status or revision. The provider interfaces and bounded audit/retry/repair runner are unchanged.

Campaigns start at revision 1. Create/get/segment-write responses include revision and an ETag such as `"campaign-42-r7"`. Message responses expose campaign_id, revision, status, messages and validation, with the same campaign concurrency token in ETag. Each successful segment/message write increments revision once, even when resubmitting the same text. Supply exactly one strong token for the target campaign in If-Match on PUT segment/messages and POST generate-segment. Wildcards, weak tokens, lists and noncanonical/overflowing values are rejected. A stale write returns 412 campaign_revision_conflict; a missing precondition returns 428 revision_required. A PostgreSQL serialization conflict returns 409 concurrent_update. Fetch and reconcile; never automatically attach a fresh token to old generated or edited content.

To save a preview or human edit, send the complete selected-channel set:

```http
PUT /campaigns/42/messages
Content-Type: application/json
If-Match: "campaign-42-r7"

{"messages":{"sms":{"body":"Merhaba {{first_name}}!"},"email":{"subject":"Offer","body":"An offer for {{city}}."}}}
```

Success returns HTTP 200:

```json
{"campaign_id":42,"revision":8,"status":"content_ready","messages":{"sms":{"body":"Merhaba {{first_name}}!"},"email":{"subject":"Offer","body":"An offer for {{city}}."}},"validation":{"revision":8,"audience":{"as_of":"2026-10-22T09:11:00Z","eligible_unique":2,"by_channel":{"sms":{"count":2,"max_first_name_runes":10,"max_city_runes":12},"email":{"count":2,"max_first_name_runes":10,"max_city_runes":12}}}}}
```

Counts/timestamps above are illustrative. For a generated candidate, construct If-Match from its base_revision, not from a later campaign read. GET messages returns the same saved representation; before any save, messages and validation are null. After a segment change, messages retain their exact text, validation becomes null and status becomes draft. Explicitly PUT the retained or edited set again to revalidate it. Invalid/stale saves preserve the entire previous set, validation metadata, status and revision.

PutMessages uses a short repeatable-read transaction: lock campaign, compare revision, compute current consent-filtered audience bounds, reuse ValidateMessages, replace all channel rows, store the validation preview, increment revision and set content_ready, then commit. GetMessages reads campaign, templates and recorded validation in one repeatable-read snapshot. GenerationInput reads campaign and audience together, closes its transaction, then calls the provider. Generated segment saves compare the original revision again after generation. No transaction or lock spans an LLM request or SSE connection.

Migrations 005/006 add monotonic revisions, template rows and validation metadata; prior migrations remain unchanged. SQL constraints protect channel/subject shape and readiness metadata. Raw template ceilings account for repeated placeholders; rendered limits remain enforced in Go against both independent audience maxima. No generated/human provenance or full content history is claimed. content_ready records successful content validation, including schema-bound empty-channel previews; future approval must revalidate current audience/content and reject an empty audience. Profile/consent changes do not advance campaign revision or turn the stored validation preview into an approval snapshot.

See [postman/README.md](postman/README.md) for the local generate/save/edit/revalidate walkthrough.

## Tests and checks

Install Go 1.27, GNU Make and Node 22.12+ (or Node 24+); Docker is needed for integration tests. Install pinned frontend tooling once with `npm --prefix web ci --ignore-scripts`.

```sh
make test
make check
make test-integration
make test-web
```

Browser tests exercise the running Compose stack with mock LLM/delivery and synthetic briefs only. Install Chromium once with `npm --prefix web exec -- playwright install chromium`, then run `npm --prefix web run test:e2e`. They cover the complete two-channel journey to terminal delivery, a competing HTTP save producing a UI 412/reconciliation flow, and mobile overflow. Run against a mock/test stack, never an Anthropic configuration. Existing backlog can extend completion times; run browser and Postman journeys sequentially for predictable timing. Screenshots stay in ignored web/test-results.

Unit tests require no database, external API or API key. Initial downloads of pinned modules require registry access. `make test` runs Go and Vue tests. `make check` also runs safety/format checks, `go vet` and strict Vue/TypeScript checks. `make test-web` builds the production frontend and runs its tests. The scanner rejects tracked secret paths without reading their contents and checks safe source files, including TypeScript/Vue/CSS/HTML, for known key patterns and hidden Unicode; matching values are never printed.

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
npm --prefix web run typecheck
go test -mod=readonly ./...
npm --prefix web test
```

For agent-run Compose checks use `docker compose --env-file .env.example ...` or set `COMPOSE_DISABLE_ENV_FILE=1` to avoid automatic reads of user secrets. The user launch command remains plain `docker compose up`.

## Migrations

Numbered `migrations/*.sql` files are embedded at build time. One startup transaction creates the ledger, applies pending files in filename order and records their names. Applied files must not be edited; use ascending, zero-padded prefixes for new files. Failure rolls back and prevents API startup. Only one API instance is supported; coordination and down migrations are intentionally absent.

## Verification status

Executed successfully on 2026-10-21: Go unit tests, vet, safety/format checks, module checksum verification, Docker image builds, Compose configuration validation and plain `docker compose up` (agent disabled automatic dotenv reads). PostgreSQL integration tests passed, covering apply/repeat/rollback. HTTP probes observed `/health=200`, `/ready=200`; with PostgreSQL stopped, `/health=200` and `/ready=503`; readiness recovered to 200 after restart. A deliberate liveness-status mutation made the endpoint test fail, and the source was restored.

Slice 2 checks executed on 2026-10-21: `make check` passed (safety/formatting, vet and all unit tests); `make test-integration` passed against local PostgreSQL. Docker rebuilt and upgraded the existing database; aggregate SQL confirmed `5000` users, one seed marker and two migration records. HTTP smoke checks observed create=201, segment save=200, preview=200 and invalid-field rejection=422 with the prior stored segment preserved. The sample preview returned 727 distinct eligible recipients (SMS 467, email 571); these are illustrative, time-dependent counts. A deliberate consent-filter mutation made `TestLLMOutputNeverReachesSQL` fail; the original source was restored and the test passed.

Make is now available. Race testing remains blocked by missing CGO/compiler support. The bundled skill validator passed (`Skill is valid!`) using verified, pinned PyYAML 6.0.3 installed only in a temporary tooling directory; no Python dependency was added to the project. The skill's description limits were also checked separately. These are historical foundation results; current submission coverage is recorded in [docs/SUBMISSION_REVIEW.md](docs/SUBMISSION_REVIEW.md).

## Known gaps

- The Claude adapter is tested against local protocol fixtures; live model access, latency and account-specific compatibility remain unverified. The default mock segment generator always returns the sample Istanbul/cart/no-purchase fixture and does not interpret arbitrary briefs. The UI states this limitation.
- The delivery dashboard polls every 2 seconds, backing off to 5 seconds on errors; push updates are not required by the assignment. System metrics refresh on demand. Current database records do not support complete per-attempt outcome history or provider-latency metrics; those proposed metrics are deferred rather than estimated. SMS segment estimates describe the template only; personalization can change encoding/segments.
- This is a local demonstration without authentication or authorization. The UI has no persistent unsaved-draft storage, campaign deep links, campaign cancellation controls, or offline mode. After a conflict it retains the local draft read-only until explicit discard/reload; it never rebases and retries automatically. Browser navigation/reload discards unsaved edits.
- Channel dispatch is serialized across processes and deliberately waits until each HTTP response plus 10 ms; slow calls reduce throughput. PostgreSQL advisory locks require direct/session-preserving connections (no transaction-pooling proxy). Default lease recovery may take 30 seconds. Provider acceptance and worker acknowledgement are separate commits: an accepted but unacknowledged final attempt can be reported dead after a crash. The acceptance ledger prevents duplicate simulated effects; it is not proof of delivery by a real provider. No real delivery provider is supported.
- Revisions protect campaign writes, not mutable profiles/consents; validation snapshots are historical. Full content history, authenticated editor identity and generated-content provenance are absent. A lost save response requires GET/reconciliation, not blind retry with a fresh revision.
- SDK malformed SSE event-envelope JSON currently maps to unavailable rather than the tool-validation repair path. Transient retries use a fixed 100 ms delay; up to six provider attempts share one generation deadline. Real-model repair effectiveness and timing remain unverified.
- PostgreSQL remains the queue choice (ADR-006).
- Local race testing lacks a CGO compiler.
- Secret scanning detects common formats, not all possible secrets, and scans working-file contents rather than staged blobs. Review the staged diff independently before a future commit. Claude hooks have not been exercised in a Claude session. The required session scrubber was executed successfully during submission review; raw exports remain ignored.
- Development used Codex; the author confirmed that no separate Claude Code/claude.ai sessions exist. Sanitized public Codex transcripts disclose omitted tool traffic and injected context. They are not fabricated Claude exports or a complete machine-event archive.
- Domain/delivery limitations remain in ADR-002/005/014/024: abandonment approximation, SMS segment estimates, final consent race, direct PostgreSQL sessions and conservative rendering bounds. ADR-024 supersedes the earlier single-owner-per-channel assumption.
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

The 3A-3C sections record their original slice behavior and verification. Current revision preconditions, editable states and persistence contracts are defined above in Campaign API and Saved messages and editing.

POST `/campaigns/{id}/generate-segment` with no body uses the stored draft brief and replaces its segment only after successful validation and call recording. The response is the same campaign representation as GET. Repeat calls keep the same deterministic segment; each invocation creates fresh call records. Concurrent draft edits use last completed save; draft state is rechecked under lock at save time. There is no database lock held during generation. Preview remains a separate request.

`LLM_PROVIDER` defaults to `mock`; `anthropic` selects the 3C adapter and is refused under `APP_ENV=test`. No key is needed or read in mock mode. `LLM_TIMEOUT` defaults to `10s` and must be positive and at most `30s`; it bounds the entire generation sequence. `LLM_MOCK_SCENARIO` defaults to `happy`; supported segment scenarios are `happy`, `unknown_field`, `extra_keys`, `no_tool_use`, `refusal`, `timeout`, `rate_limited`, `repair_succeeds` and `repair_exhausted`. These are startup/test controls, never request fields. Compose selects `happy`. The mock returns fixtures independently of brief wording; it is a test adapter, not a language parser.

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

## Mock message generation and streaming (3B)

POST `/campaigns/{id}/generate-messages` with an empty body loads a draft with a segment and computes one consent-filtered aggregate audience preview. No recipient records enter the prompt. Tone comes from the brief; the deterministic mock does not interpret it. Messages remain a preview: nothing is saved and campaign state remains draft. Empty audiences may generate previews but cannot be approved in a later approval flow. A zero-count selected channel uses schema bounds (first_name 80, city 120) rather than zero-length substitutions.

The response uses `text/event-stream` and `Cache-Control: no-store`. Consume this POST with streaming fetch, not the GET-only EventSource constructor. Each frame has an `event:` line and a JSON `data:` line followed by a blank line:

- `attempt`: `{ "attempt": 1 }`. Discard all prior displayed deltas whenever a new attempt starts, including transport retries.
- `delta`: `{ "attempt": 1, "channel": "sms", "field": "body", "text": "Merhaba " }`. Append plain text to that field for the current attempt only. Display using textContent, never HTML; partial output is provisional and must not be approved.
- `validated`: terminal `{ "attempt": 1, "messages": { ... }, "audience": { ... } }`. Contains the complete validated templates for exactly the selected channels and the aggregate preview used for validation. Replace displayed deltas with this payload.
- `error`: terminal `{ "attempt": 1, "error": { "code": "llm_output_invalid", "path": "$", "reasons": [ ... ] } }`. Discard provisional output. Error reasons contain codes/paths only.

Request/state/database failures before streaming use the normal JSON error contract (404/409/413/422/503). Once streaming begins, HTTP remains 200 and generation errors use the terminal event, so status alone does not mean success. A closed or interrupted stream without a terminal event is a failure. Disconnects cancel the provider; failed writes stop generation. The generation deadline bounds the entire sequence and the stream has an absolute write deadline. Metadata auditing retains the bounded cancellation-cleanup budget from 3A. There is no automatic resume or SSE event history.

Message JSON rejects duplicate/case-mismatched/unknown keys, unselected or missing channels, nulls, invalid types and UTF-8, NULs, malformed/unknown placeholders and trailing JSON. Every field is nonempty. The validator counts rendered runes using BOTH maximum first_name and city lengths together for every occurrence. SMS body is limited to 160, email subject to 80, email body to 2000. These are preview-time bounds, not a frozen approval snapshot: later persistence/approval must revalidate current content, audience and consent. SMS encoding/segment estimates are deferred with persistence/UI; the hard assignment limit is enforced.

`LLM_MOCK_MESSAGE_SCENARIO` defaults to `happy`; supported scenarios are `happy`, `sms_too_long`, `subject_too_long`, `unknown_field`, `unknown_variable`, `extra_keys`, `no_tool_use`, `refusal`, `timeout`, `rate_limited`, `repair_succeeds`, `repair_exhausted`. `LLM_MOCK_CHUNK_DELAY` defaults to `10ms` (0s to 1s). The mock emits eight-rune chunks before final validation. These are startup/test controls, never request fields. Segment generation retains its separate scenario setting. No API key, SDK, new dependency, real API or delivery is used. Migration 004 expands the existing audit call-name constraint to include GenerateMessages; prior migrations remain unchanged.

### 3B verification (2026-10-22)

Executed `make check` (repository safety/formatting, vet and all unit tests), `make test-integration` (local disposable PostgreSQL), Compose API rebuild and the documented local Postman collection command. Postman passed 49 requests and 127 assertions, zero failures, exit 0. It checked SSE framing, reconstruction of final messages from current-attempt deltas, both rendered maxima, repeat generation, missing segment and rejected request preservation. PostgreSQL tests verified migration 004, per-attempt GenerateMessages audit records and unchanged draft/segment, including repair exhaustion, timeout and audit loss.

Actual check output:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

Go packages passed without an API key. Real-socket HTTP tests separately proved flushing before provider completion and disconnect cancellation. The initial sandbox run blocked loopback connections; the same tests passed with approved local-network execution. No tests were skipped. A deliberate omission of city maxima failed SMS, repeated-variable, subject, email-body and zero-count bound tests. Disabling Flush failed `TestSSEFlushesBeforeGenerationCompletes` with a client timeout awaiting headers; sources were restored before final checks.

Safety coverage: message boundary/length rules are covered by `TestMessageValidation`; Rule 1 and strict output/repair boundaries by `TestMockMessageScenarios`, existing segment tests and the shared runner; Rule 3 by the existing consent-aware `TestDomain`; Rule 5 by metadata-only call records, safe error payloads and repository scanning; Rule 6 by the mock-only provider guard and key-free tests; Rule 9 by both deliberate failing controls. Cancellation, transport errors and preview-only state are exercised by the HTTP and PostgreSQL tests. No dependency, real external API, recipient contact output, message persistence, approval or delivery was introduced. Race testing remains blocked by the existing missing CGO compiler.

## Claude adapter (3C)

The existing generation endpoints select a provider at startup. Default `LLM_PROVIDER=mock` remains key-free. To opt into Claude, supply `LLM_PROVIDER=anthropic`, `ANTHROPIC_MODEL` and `ANTHROPIC_API_KEY` through your runtime environment or private local Compose configuration, then rebuild/recreate the API. Use `APP_ENV=development` or `production`; construction under `APP_ENV=test` is rejected before looking up a credential. No model/key is required in mock mode. `LLM_TIMEOUT` bounds each complete generation sequence as before; slower live calls may need a larger value up to 30s. No live Claude call is made by the documented automated tests.

Only internal/llm imports the official SDK. Version 1.82.0 was verified through proxy.golang.org metadata: its origin is anthropics/anthropic-sdk-go, tag v1.82.0, commit 543d266ce81a4487281e084b013fef807dbb5f18 (2026-10-15). The upstream-pinned transitive modules and checksums are recorded in go.mod/go.sum; no cloud-provider SDK is imported by application code. Disable SDK environment/profile credential discovery and OpenTelemetry content tracing. The adapter reads only the explicitly supplied environment key and fixes the endpoint to HTTPS api.anthropic.com; it ignores ambient SDK base-URL/auth/profile/custom-header settings and refuses redirects. Never log or print the key, SDK errors, HTTP dumps or resolved Compose configuration. The scratch image includes the CA certificate bundle from the pinned Go builder image.

Each request supplies exactly one forced tool, the versioned schema, a token bound and safe validator feedback. Disable SDK retries to preserve the runner's one transient retry per validation attempt and at most two repairs. Nonstreaming segments accept exactly one expected tool block with tool_use stop reason. Message streams require ordered blocks, one expected tool, completed input, and terminal message_stop; max_tokens, missing/wrong/multiple tools and malformed/truncated streams cannot produce a validated result. Refusals remain terminal; 429, 5xx and overloaded errors follow the existing bounded retry path. Error bodies and raw SDK errors are discarded; audit rows receive observed token counts and safe outcome codes.

Tool JSON deltas are accumulated within 32768 bytes. The decoder emits only provisional sms.body/email.subject/email.body string prefixes, handling escaped quotes, split Unicode escapes and surrogate pairs. Full raw input is preserved for final strict domain validation, so canonicalization never hides duplicate keys. Only final validated payloads can be used as complete previews; no message persistence or approval was added. The decoder rereads a bounded prefix per upstream delta, appropriate for this case study's small document limit. HTTP responses are limited to 1 MiB, recognized stream events to 8192, and content blocks to 16, with context/write deadlines retained.

SDK tests use private local HTTP servers and fake credentials, rather than replacing SDK methods with stubs. They exercise actual request serialization, streaming consumption, cancellation, retries, repairs, refusal/truncation and token accounting. Test fixtures are synthetic and follow Anthropic's documented Messages/SSE protocol, not recordings from a real account. These establish local protocol compatibility; they do not establish that the configured live model or key is available. Postman continues to run only against the mock/test Compose API, with no external Claude request. No new endpoints, API response fields or migrations were required.

### 3C verification (2026-10-22)

Executed successfully: `make check`; `go mod verify` (`all modules verified`); local SDK/decoder tests; `make test-integration` against disposable PostgreSQL; mock/test Compose rebuild including the CA certificate copy; and the documented local Postman run. Postman passed 49 requests and 131 assertions, zero failures, exit 0. The API was explicitly launched with LLM_PROVIDER=mock and APP_ENV=test, with any inherited key cleared without reading it. No real Anthropic call or real credential was used.

Actual check output included:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
all modules verified
```

The Go packages passed. Tests exercised actual SDK HTTP requests to local synthetic servers, not SDK method stubs: forced tool/model/token/schema serialization, safe feedback repairs, exact retry/audit counts, 400/401/403/429/500/529 failures, stream-overload events, refusal, missing/wrong/multiple tools, duplicate JSON keys, max-token/truncated/missing-stop streams, token counts, timeouts and cancellation after a display delta. Decoder tests cover byte-by-byte fragments, quotes/backslashes/newlines, UTF-8, split Unicode escapes/surrogates, duplicate fields, unknown display paths and document bounds. An intentional removal of the final terminal-stop rejection made `TestClaudeStreamTruncationAndErrors/missing_stop` fail with `bad stream accepted <nil>`; the source was restored before final checks.

Rule 1/boundary validation is covered by SDK invalid-output tests plus existing DSL/message validators; Rule 5 by SDK error redaction, explicit credentials/destination bounds, disabled environment/profile loading and tracing, and metadata-only audit rows; Rule 6 by independent config and constructor guards (before key lookup), fake local fixtures and mock/test Postman; Rule 8 by registry/tag origin, pinned module/checksums and `go mod verify`; Rule 9 by the deliberate failing terminal-stop control. Existing consent and repeat-safe storage tests passed. A dependency-graph inspection found no cloud-provider SDK imports in the API. No RULES.md changes, real delivery, commits or new public endpoints/migrations were introduced.

During verification, the first cancellation fixture did not consume its request body and could hang awaiting disconnect; the fixture was fixed to drain it, and now asserts a display delta preceded cancellation. An interrupted test process was checked and none remained. Sandbox network/cache restrictions required approved local-network/module-cache execution; tests were not skipped. Race testing remains blocked by the existing missing CGO compiler. Live model/key availability, public TLS connectivity, account permissions and production latency remain unverified.


## Slice 3D verification (2026-10-22)

Implemented and checked incrementally: separate compatible-model correction, campaign revisions/conditional segment writes, then message persistence/editing and coherent streaming snapshots. Existing uncommitted Slice 3C work was preserved. No new dependency, RULES.md change, commit, real Anthropic call, approval or delivery functionality.

Executed make check successfully after implementation and the restored deliberate failure control:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

The Docker PostgreSQL suite passed against guarded scc_test, including migrations from 004 with an existing draft, repeat/rollback, competing segment writes, message/edit versus segment races, all-or-nothing replacement, invalid/stale rejection, current profile/consent bounds, single channels, cancellation and preserved text with invalidated readiness. A raw 2240-character placeholder SMS that renders to 160 characters persisted and round-tripped successfully. Existing domain/LLM/audit/seed regressions remained green. The Compose mock/test API rebuilt and applied migrations 005/006. Final Postman execution passed 74 requests and 176 assertions, zero failures, exit 0, including regeneration from content_ready; reporting stayed local.

TestRevisionPreconditions failed with `wrong precondition error <nil>` after temporarily accepting a missing If-Match header. The source was restored; final checks passed. Initial sandbox loopback failures were rerun with approved local access, without skipping tests. Race-detector execution remains unavailable with the existing missing CGO compiler; synchronized PostgreSQL concurrency tests did execute. No real model/account compatibility is claimed.

Rules 1/3 and rendered-bound validation are exercised by existing TestDomain/TestLLMOutputNeverReachesSQL plus TestMessageSaveUsesCurrentAudienceBounds. Strict write/revision/state boundaries are exercised by TestMessagePersistenceHTTP, TestRevisionPreconditions, TestSlowGeneratedSegmentCannotOverwriteEdit, TestSavedMessagesLifecycle and PostgreSQL concurrency/rollback tests. Rules 5/7 remain supported by safe errors, metadata-only logs, the repository scanner and explicit safe Compose template. Rule 6 remains protected by mock/test mode and constructor/config guards. Rule 9 is supported by the deliberate failing precondition control.

## Campaign approval and PostgreSQL queue

Generate -> validate preview -> explicitly save/edit -> revalidate -> content_ready -> review current audience preview -> approve and atomically enqueue -> sending -> completed. The intermediate approved state is not exposed. The worker consumes pending deliveries as described below.

POST `/campaigns/{id}/audience-preview` with no body after saving messages. Retain its ETag, revision and non-null approval_fingerprint together. The SHA-256 fingerprint covers versioned canonical templates, campaign revision, ordered recipient/channel membership, destinations and literal rendered payloads. It excludes as_of, so elapsed time alone does not force another review; event-window membership changes do. It is a comparison token, not authorization. A draft or empty audience has no approval fingerprint. A zero-count selected channel is allowed when the union is nonempty; all selected templates still undergo existing schema-bound validation.

```http
POST /campaigns/42/approve
Content-Type: application/json
If-Match: "campaign-42-r8"

{"approval_fingerprint":"<64 lowercase hexadecimal characters from the reviewed preview>"}
```

The placeholder above must be replaced with the actual preview value. Example HTTP 200 body for two recipients, one per channel:

```json
{"campaign_id":42,"revision":9,"status":"sending","approved_at":"2026-10-22T09:15:00Z","approved_revision":8,"counts":{"total":2,"pending":2,"in_flight":0,"sent":0,"dead":0,"skipped":0},"by_channel":{"sms":{"total":1,"pending":1,"in_flight":0,"sent":0,"dead":0,"skipped":0},"email":{"total":1,"pending":1,"in_flight":0,"sent":0,"dead":0,"skipped":0}}}
```

HTTP -> strict JSON/precondition validation -> Campaigns.Approve -> bounded repeatable-read transaction -> campaign row lock/revision comparison -> current consent-aware segment queries -> revalidate complete saved templates against current independent bounds -> literal, nonrecursive personalization -> compare reviewed fingerprint -> insert frozen jobs in bound batches of 200 -> record approval metadata, increment revision once and set sending -> commit. Any failure rolls back all batches and campaign changes. No external call or SSE connection is inside this transaction. One unique row per campaign/user/channel and a deterministic unique idempotency key protect queue insertion. Migration 007 adds approval metadata, frozen destination/subject/body, pending state, attempt/due/lease/generation fields and a partial due-job index; those lifecycle fields do not implement a worker.

An empty union returns 409 empty_audience. Changed membership, destination or rendered content returns 409 approval_preview_changed with a safe replacement preview, even when aggregate counts are unchanged; review again before resubmitting. Current bound violations return the existing 422 validator code. Invalid state returns 409, stale revision 412, missing precondition 428, malformed JSON/token 400, oversized body 413, invalid fingerprint 422, missing campaign 404 and database timeout/unavailability 503. Serialization conflicts return 409 concurrent_update. All rejected attempts preserve the queue and revision.

A successful approval returns the new ETag. After a lost response, GET campaign to reconcile its current revision. Repeating approval with that current token and the same original fingerprint returns the existing receipt/counts without creating jobs or advancing revision; the old token remains stale. Another fingerprint on an already approved campaign is rejected. Segment/template edits and generation cannot mutate approved content.

GET `/campaigns/{id}/delivery-status` returns the response shape above for the current PostgreSQL snapshot, including all selected channels and zero-valued counters. Before approval the approval fields are null and counts zero. It exposes no user IDs, destinations or payloads. It deliberately has no ETag because delivery progress changes without advancing campaign revision; GET campaign supplies the mutation token. No cancellation, worker control, enqueue-again or recipient-detail API is added.

Consent/profile data is coherent at the approval transaction snapshot. Dispatch rechecks consent immediately before each mock send and uses frozen destinations/content. The worker and durable mock ledger provide the retry/recovery behavior described below; queue uniqueness alone does not prove exactly-once delivery.

### Approval/queue verification (2026-10-22)

Executed make check, Docker disposable-PostgreSQL integration tests, rebuilt the mock/test Compose API (migration 007 applied), and ran the local Postman collection: 97 requests, 232 assertions, zero failures, exit 0. Actual check output:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

Tests cover current consent/bounds, same-count membership/profile/contact changes, frozen literal payloads, duplicate prevention, reconciled repeats, stale revisions, edit/approval and approval/approval races, rollback across insertion batches, cancellation, aggregate status privacy and the 5000-user seed under the existing two-second database deadline. Temporarily making substitution recursive caused TestLiteralMessageRendering to fail; the original source was restored before final checks. No real Anthropic or delivery calls, new dependencies, RULES.md edits or commits. Race-detector coverage remains unavailable with the existing missing CGO compiler; synchronized database concurrency tests ran.

Rules 1/3 are exercised by existing validated segment compilation plus TestApprovalDetectsSameCountChanges and TestApprovalRejectsInvalidCurrentState. Rule 4 queue uniqueness/repeat protection is covered by TestApprovalQueueSnapshotAndRepeat and TestApprovalConcurrency; worker-side delivery deduplication is not yet implemented. Rule 5 is covered by aggregate-only HTTP/status assertions, private storage payloads and the repository scanner. Rule 6 retains mock/test mode and constructor guards. Rule 9 is backed by the deliberate rendering failure control. Review of the actual implementation diff: No findings.

## PostgreSQL worker and mock delivery

Run `docker compose up --build`. The API applies migrations and seeds fake users; the separate `cmd/worker` process consumes jobs and `cmd/mockprovider` accepts simulated HTTP messages. Existing pending campaigns are processed automatically. No LLM, segment compilation, recipient lookup or personalization occurs during delivery. The only mutable audience-related lookup is the final consent/campaign/claim check.

Queue lifecycle preserves the API vocabulary:

- pending -> in_flight: atomically claim one due row with FOR UPDATE SKIP LOCKED, increment attempts and claim_generation, set locked_at and lease_expires_at.
- in_flight -> sent: provider accepted the frozen payload and returned its deterministic receipt.
- in_flight -> pending: network/timeout, 429, 5xx or malformed success receipt; schedule capped exponential full-jitter backoff and respect Retry-After as a minimum.
- in_flight -> dead: permanent rejection or the fifth failed/abandoned processing attempt.
- in_flight -> skipped: final consent is absent/revoked or the campaign is no longer sending.

Claims count processing attempts, including abandoned ones. The maximum is five, enforced by worker/repository policy and a database constraint. Backoff is a uniform delay from zero through min(60s, 1s * 2^attempts); a larger Retry-After is honored. Expired leases are recovered on worker polling and become immediately due, or dead at the limit. Graceful shutdown stops claiming, cancels waits/HTTP, waits for goroutines and leaves ambiguous work leased for recovery. No new administrative retry/cancellation API is introduced.

A result transaction locks the campaign before its delivery row, compares the active generation and unexpired lease, writes the outcome, then sets completed only when no pending/in_flight rows remain. Recovery follows the same lock order. Claims skip locked delivery rows; results from a superseded worker cannot overwrite newer claims. Completion can contain dead/skipped rows and never increments campaign revision. GET delivery-status keeps its existing aggregate shape; counters now advance asynchronously.

All processes share a PostgreSQL session advisory lock per channel. While waiting, a live worker renews its claimed lease. Admission waits for the durable next-start time, checks consent/ownership immediately before HTTP and extends the lease. Before sending it reserves admission through that lease; normal cleanup shortens the reservation to response completion plus 10 ms. This provides conservative cross-process pacing without burst credit, including retries; a hard crash leaves a durable cooldown. Calls are context-bounded by the provider timeout and lease deadline. No worker transaction spans HTTP. A session connection is held through dispatch; uncertain unlock/acquisition failures discard it rather than returning a locked session to the pool. Wall-clock lease deadlines assume the worker and PostgreSQL clocks are synchronized, as in local Compose.

The mock validates limits and permits only mock-phone-* destinations and @example.test emails. POST /messages receives a frozen channel/destination/subject/body and Idempotency-Key. It returns {"message_id":"mock-<64 hex characters>"}, or safe 422/409/429/503 errors. Its PostgreSQL acceptance ledger stores only the deterministic key, payload hash, channel, receipt and timestamp. Matching accepted retries return the same receipt; conflicting content is rejected. Acceptance survives provider restart and ambiguous worker acknowledgement. The mock independently enforces a 100-per-second rolling admission limit, returns Retry-After: 1 on 429, and derives faults/delay from FAULT_SEED, key and persisted attempt number. There is no real messaging SDK or forwarding path.

GET http://127.0.0.1:8081/debug/messages exposes only {"total":N,"by_channel":{"sms":N,"email":N}}. /health and /ready are available on the mock provider; the API retains its existing health endpoints. Neither debug output nor logs expose destinations/message content. Provider URL configuration accepts only HTTP on mockprovider or explicit loopback IPs with a valid port; redirects and ambient proxies are disabled.

Configuration (names also in .env.example): WORKER_CONCURRENCY is 1..32 per channel, default 1; WORKER_LEASE defaults to 30s, WORKER_POLL to 100ms, PROVIDER_TIMEOUT to 3s. The lease must exceed provider timeout plus twice DB_TIMEOUT plus one second. The worker pool reserves extra connections for acknowledgements/recovery. More concurrency/replicas does not raise the shared per-channel dispatch limit. FAULT_RATE_5XX and FAULT_RATE_429 default in Compose to 0.05 and 0.02; their sum must not exceed one. FAULT_LATENCY_P95_MS is the retained configuration name for the deterministic delay upper bound (default 20 ms), not a measured percentile. No authentication or public exposure is added to the local mock.

Local checks and controls (PowerShell, without reading private configuration):

```powershell
$env:COMPOSE_DISABLE_ENV_FILE='1'
$env:APP_ENV='test'
$env:LLM_PROVIDER='mock'
$env:ANTHROPIC_API_KEY=$null
$env:FAULT_RATE_5XX='0'
$env:FAULT_RATE_429='0'
$env:FAULT_LATENCY_P95_MS='0'
docker compose --env-file .env.example up -d --build
docker compose --env-file .env.example stop worker
docker compose --env-file .env.example start worker
```

Stop the worker to inspect pending jobs; start it to observe progress. To exercise hard-stop recovery locally, use docker compose --env-file .env.example kill -s SIGKILL worker, then start worker; abandoned leases become recoverable after their deadline. The worker and mock provider may be restarted independently. Do not delete volumes to simulate recovery: the queue and acceptance ledger must survive.

Run the maintained Postman collection with the local mock/test API and worker active. The final request polls delivery-status for up to 180 seconds, checks completion, conservation of job counts and unchanged revision. Request/assertion totals vary with polling. For deterministic end-to-end success, use the zero-fault overrides above; failure behavior is exercised separately in Go/PostgreSQL tests. A large existing backlog can need more than the polling allowance. Stop workers before running older historical queue-only workflows.

### Worker/mock verification (2026-10-22)

Executed make check successfully (repository safety/formatting, go vet and all unit tests), the full Docker disposable-PostgreSQL integration suite, mock/test Compose startup and the complete local Postman collection. Final Postman execution passed 203 requests and 548 assertions, zero failures, exit 0, in 2m 6.5s; totals include repeated progress polling and the mock health/ready/debug/invalid-ingress contracts. Two worker containers ran concurrently during this execution. Earlier execution also passed (217 requests, 593 assertions); counts differ with polling/backlog. Cloud reporting was disabled.

Actual Go check output included:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

The PostgreSQL tests cover successful completion/revision preservation; transient success on retry, permanent rejection and exhaustion; atomic competing claims and two concurrent workers; acceptance-before-ack crash simulation, stale dispatch/result fencing and provider restart deduplication; claim/result/provider transaction rollback; consent revocation during admission and consent-query failure; independent provider rate rejection; 104 concurrent/repeated channel admissions with rolling-window checks; waiting-lease renewal and cancellation. Existing approval, migration, template, seed and LLM/audit regressions passed. No tests were skipped. Database-authoritative timing is checked with PostgreSQL timestamps and controlled SQL expiry/aging, rather than a fake process clock for the shared gate.

A separate Compose SIGKILL smoke test observed an abandoned active lease, restarted worker and mock provider, then completed all 1597 jobs with unchanged campaign revision. A SQL aggregate comparison confirmed total mock acceptances equalled sent rows. Graceful stop/start was also exercised. No recipient payload or destination was printed. Unit tests assert cancellation does not acknowledge ambiguous work. Temporarily accepting permanent HTTP failures as successful responses made the 400/401/409/422 classification regressions fail; the source was restored and checked green.

Safety coverage: Rules 1/3 retain the trusted compiler and current-approval regressions, with TestFinalConsentAfterGateWait and TestConsentAndProviderStorageFailClosed proving final dispatch boundaries. Rule 2 is covered by TestRetryPolicyAndMockClient allowlist checks and TestMockIngressAndLimits synthetic destination checks. Rule 4 is covered by TestConcurrentClaimsAndWorkers, TestDeliveryCrashFencingAndProviderRestart and the hard-stop aggregate check. Rules 5/7 use metadata-only logs, aggregate HTTP contracts and the repository scanner; no protected configuration was read. Rule 6 retains mock/test mode and the existing real-client construction guard. Rule 9 has the deliberate classification failure control. No new dependency, RULES.md edit, real Anthropic/SMS/email call or commit. Sandbox loopback/Docker denials were rerun with approved access, without skipping tests. Race-detector execution remains unavailable with the existing missing CGO compiler. Actual implementation diff review: No findings.

## Frontend/dashboard/metrics verification (2026-10-22)

Implemented the approved milestone with Vue 3, strict TypeScript and Vite, a typed fetch API client, cancellable POST SSE, explicit template saves, reviewed revision/fingerprint approval, responsive list/detail screens, polling delivery counts and aggregate metrics. Compose builds and serves the frontend on port 5173. No existing mutation contract, queue algorithm, schema or Go dependency changed.

Executed successfully:

- Production frontend build and strict vue-tsc check, locally and in the pinned Node/nginx Docker image.
- make check: repository safety/format scan, go vet, all Go unit/HTTP regressions, strict TypeScript and 17 frontend tests across 5 files.
- Full Docker PostgreSQL integration suite, including cursor stability under concurrent creation, empty pages, persisted LLM/error aggregates and pending/terminal delivery aggregates; existing delivery/approval/consent/retry/migration tests remained green.
- Full local mock/test Postman run: 217 requests and 582 assertions, zero failures, exit 0. Totals include completion polling. After final pagination hardening and Docker recovery, the affected dashboard folder passed 4 requests/4 assertions again.
- All 3 Playwright Chromium tests on the final rebuilt frontend: complete two-channel journey through terminal delivery; real competing HTTP save causing 412 and explicit UI reconciliation; 390-pixel mobile layout without horizontal overflow. Final run: 3 passed (26.7s). The earlier full browser run also passed while sharing the mock queue with Postman.
- Desktop and mobile screenshots visually inspected. The final synthetic campaign completed 1,010 jobs, all acknowledged by the mock provider, with zero pending/in-flight/dead/skipped jobs. Counts are time-dependent seed observations, not fixture constants.
- git diff --check and a separate review of the actual implementation changes. No findings remain.

Actual final check output included:

```text
repository safety and formatting checks passed
npm --prefix web run typecheck
> vue-tsc --noEmit
go test -mod=readonly ./...
Test Files  5 passed (5)
     Tests  17 passed (17)
```

Frontend tests cover split UTF-8/CRLF framing, repair-attempt resets, incomplete/error/invalid streams, cancellation, explicit review, stale-save preservation without automatic retry, changed fingerprints, incoherent snapshots, empty audiences, polling completion/retry/cleanup, and GSM-extension/Unicode SMS estimates. Temporarily removing the explicit human-review guard made the corresponding test fail; restoring the source made it pass. No failing test was skipped or weakened.

Safety coverage: Rules 1/3/4 retain the compiler, consent, revision, approval and queue regressions. Rules 2/6 retain mock/test startup, mock-only provider transport and real-client test construction guards; browser and collection runs used synthetic briefs with the mock LLM/provider. Rules 5/7 are covered by fixed-label metrics, aggregate-only HTTP/Postman checks, safe error tests and expanded frontend source scanning. Rule 8 is covered by verified npm maintainer/repository metadata, exact dependency pins/lockfile and official image digests (ADR-025). Rule 9 includes the deliberately failing review-guard control. No real Anthropic, SMS or email request was made.

Environment failures were handled separately: sandbox loopback tests passed after approved local-network access; an unsupported Postman folder flag was corrected to the documented item selector. On session continuation Docker's engine was unavailable, causing connection-refused browser/Postman runs before any application assertion. After Docker recovered and /api/ready returned ready, both suites passed. No database volume was reset and no application behavior was changed to hide these failures.

This historical milestone deferred exports, ADR reconciliation and final review; those are now recorded in [docs/SUBMISSION_REVIEW.md](docs/SUBMISSION_REVIEW.md). Live Claude verification remains unperformed and requires explicit authorization; the fixture-backed adapter and mock journey are verified. Local race detection still lacks the existing CGO/compiler requirement. No commit was created.

## PostgreSQL 18.6 verification (2026-10-25)

Verified the official image index digest with docker buildx imagetools inspect. Runtime SQL reported server_version 18.6, data_directory /var/lib/postgresql/18/docker, 8 applied migrations and 5000 seeded users on fresh storage. No application migration or driver change was required.

Executed make check (exit 0), including repository safety/formatting, go vet, frontend type checking, Go unit/HTTP tests and frontend tests. Selected actual output:

```text
repository safety and formatting checks passed
Test Files  5 passed (5)
     Tests  17 passed (17)
```

The full Docker PostgreSQL integration suite passed against disposable scc_test: internal/store 21.172s, exit 0. It exercises existing migration, consent, approval, concurrency, retry, admission and deduplication regressions on PostgreSQL 18.6. The maintained local Postman collection passed 228 requests and 615 assertions, zero failures, exit 0; all 6267 mock deliveries completed. Reporting stayed local.

A forced database container recreation preserved aggregate counts: 5000 users, 8 migrations, 2 campaigns, 6267 deliveries and 6267 mock acceptances; API /ready returned ready afterward. These are observed test-run counts, not fixed application requirements.

The first integration/Postman attempt was interrupted by Docker Desktop stopping; both were rerun after restarting it. A sandboxed make check rerun blocked local HTTP fixtures; the approved local-access rerun passed. No tests were removed or skipped. Safety rules 1/3/4 remain covered by the existing segment/consent/queue integration tests; rules 2/6 by mock-only services and test configuration; rules 5/7 by the repository safety scanner and explicit template-only Compose invocation. No real Claude call or real delivery occurred. Browser E2E tests were not rerun for this infrastructure change.
