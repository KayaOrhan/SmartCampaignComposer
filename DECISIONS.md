# Decisions

Architecture decision records for Smart Campaign Composer. Newest decisions go at the bottom. If a decision changes, do not delete the old entry: set its status to `Superseded by ADR-0NN` and add the new one.

Status values: `Proposed` (reasoned, not yet proven by code), `Accepted` (built and verified), `Superseded`.

> Draft written 2026-10-21 before implementation. Every entry starts as `Proposed`. Change each one to `Accepted` only after the code and its test exist, and rewrite any entry that reality proved wrong. A DECISIONS.md the author cannot defend line by line is worse than none.

---

## ADR-001: Stack: Go, Postgres, Vue 3 with TypeScript
Status: Proposed

**Context.** The brief requires Vue.js on the frontend and Go or PHP on the backend. Database and queue are open. The system needs concurrency (workers, rate limiting), strong typing for validated LLM output, and few moving parts to run with one `docker compose up`.

**Decision.** Go with the standard library `net/http`, Postgres for both data and queue, Vue 3 with TypeScript and Vite, Docker Compose with services `api`, `worker`, `mockprovider`, `db`, `web`.

**Alternatives.** PHP/Laravel (good queue tooling, weaker fit for a typed validation core). Redis or NATS for the queue (more services, and a second place where state can disagree with the database).

**Consequences.** Fewer dependencies and one source of truth. The Go standard library routing means a little more code than a framework. Postgres is the throughput ceiling for the queue, which is far above 100 messages per second per channel.

## ADR-002: Audience as a constrained JSON DSL, never SQL
Status: Proposed

**Context.** Claude must turn "last 30 days, Istanbul, added to cart, did not buy" into an audience. The brief forbids running LLM output as SQL.

**Decision.** Claude returns a versioned JSON DSL with an allowlist of fields and operators (see `campaign-domain`). The backend decodes it strictly, validates it, and compiles it into a parameterized query in our own code. Consent filters are added by the compiler and are not expressible in the DSL.

**Alternatives.** Let Claude emit SQL and filter it (a denylist cannot be proven complete). Let Claude emit a full query-builder AST (more power than the use case needs).

**Consequences.** The model cannot widen the audience beyond what the DSL allows or drop the consent filter. The DSL limits what marketers can express. `purchase.none_within_days` approximates "added to cart and did not buy" as "no order in the last N days", which can include someone who bought before adding to cart. Accepted for v1, listed in README Known gaps.

**Verification.** `TestLLMOutputNeverReachesSQL`, plus an injection fixture.

## ADR-003: Structured output through forced tool use, validated, with a bounded repair loop
Status: Proposed

**Context.** The output must be structured, checked in the backend, and reliable.

**Decision.** Each Claude call defines one tool whose JSON Schema is the contract and forces that tool. The result is decoded strictly, then checked semantically. A failure triggers at most 2 repair calls that include the validation errors. After that the marketer sees the reasons.

**Alternatives.** Parse JSON out of free text (fragile). Accept the first response (reliability depends on luck). Silently truncate over-long copy (can change meaning).

**Consequences.** Latency and cost can grow by up to two extra calls in bad cases. Validation failures are measurable through `llm_validation_failures_total{reason}`, which shows where the prompt needs work.

## ADR-004: Streaming to the browser over SSE, approval only after validation
Status: Proposed

**Context.** Messages should appear while they are generated, but incomplete or invalid content must never be approvable.

**Decision.** The backend streams display deltas over Server-Sent Events. A final `validated` event carries the complete payload. The UI enables Approve only after that event.

**Alternatives.** WebSocket (more machinery than one-way streaming needs). No streaming (worse experience, and the brief asks for it).

**Consequences.** Streaming forced tool calls produces partial JSON, so extracting display text mid-stream is the riskiest implementation detail. It gets a spike before the UI is built. If the spike fails, fall back to streaming per completed field.

## ADR-005: SMS length: hard limit 160 characters, with an encoding warning
Status: Proposed

**Context.** The spec says 160 characters. Turkish letters such as `ş`, `ğ`, `ı` force UCS-2 encoding, where one SMS segment holds 70 characters. The audience is Turkish.

**Decision.** Enforce the specified 160 characters as a hard limit on the rendered worst case (longest `first_name` in the audience), counted in runes. Also compute an informational `sms_segments` and show a warning when the text needs UCS-2.

**Alternatives.** Enforce 70 when non-GSM characters appear (stricter than the spec, may reject valid copy). Ignore encoding (hides a real cost for the customer).

**Consequences.** Behaviour matches the brief, and the marketer sees the real cost. The segment estimate is approximate and documented as such.

## ADR-006: Queue in Postgres with `FOR UPDATE SKIP LOCKED`
Status: Proposed

**Context.** An asynchronous queue is required. The `deliveries` rows already need durable state and uniqueness.

**Decision.** `deliveries` is the queue. Workers claim batches with `FOR UPDATE SKIP LOCKED`.

**Alternatives.** Redis list or streams, NATS, RabbitMQ.

**Consequences.** Enqueue, status and idempotency live in one transaction boundary. One fewer service in compose. Throughput is lower than a dedicated broker but well above the requirement. If volume grew by orders of magnitude we would move the queue out.

## ADR-007: Exactly-once effect from at-least-once delivery
Status: Proposed

**Context.** "The same person never gets the same message twice", while workers can crash and networks can time out after the provider already accepted a request.

**Decision.** Three layers. A `UNIQUE (campaign_id, user_id, channel)` constraint prevents duplicate rows. Every provider call carries a deterministic `Idempotency-Key`, and the mock provider dedupes on it. A reaper returns stale `in_flight` rows to `pending`, and the resend reuses the same key.

**Alternatives.** Application-level "already sent?" checks (race conditions). Treat a timeout as failed without resending (loses messages).

**Consequences.** A real provider must support idempotency keys. The mock does. Integration tests kill a worker mid-flight and assert unique accepted messages equals deliveries.

## ADR-008: Rate limiting: in-process token bucket per channel
Status: Superseded by ADR-013

**Context.** At most 100 messages per second per channel.

**Decision.** A token bucket per channel inside the worker process, waited on before each provider call. Compose runs one worker replica per channel.

**Alternatives.** Database or Redis shared limiter (needed only when scaling to several replicas).

**Consequences.** With N replicas the effective limit is N times 100, which is documented. The scaling path is a shared limiter. The mock provider enforces the limit independently and returns 429, so the client limiter is proven by test, not trusted.

## ADR-009: Retry policy
Status: Proposed

**Decision.** Retry on network errors, timeouts, 5xx and 429 (honouring `Retry-After`). Do not retry other 4xx; mark `dead`. Backoff is `min(60s, 1s * 2^attempts)` with full jitter. Maximum 5 attempts, then `dead` with the last error stored.

**Alternatives.** Infinite retry (hides permanent failures). Fixed delay (synchronized retries after an outage).

**Consequences.** Permanent failures surface on the dashboard quickly. A campaign can complete with `dead` rows, and the dashboard shows them separately.

## ADR-010: Testing without external network or API key
Status: Proposed

**Decision.** The LLM sits behind an interface. A mock serves fixture scenarios, including failures (invalid JSON shape, over-long copy, refusal, timeout, 429). Time is injected. The mock provider is a real HTTP service so integration tests exercise the real client, limiter and retry code. `APP_ENV=test` makes the real Anthropic client refuse to construct.

**Alternatives.** Record and replay real API responses (needs a key to create and goes stale). Mock at the HTTP library level only (does not test our contract).

**Consequences.** `make test` runs anywhere. Fixture scenarios double as living documentation of failure modes.

## ADR-011: Scope
Status: Proposed

**Decision.** Build the core flow end to end on mocks first, then harden, then polish the UI. Bonus items (scheduled send, A/B variants, per-user language, CI) are out unless the core is solid and verified. Cuts are written in README "Known gaps".

**Consequences.** A smaller, defensible submission instead of a wide, shaky one.

## ADR-012: How AI was used and how its output is verified
Status: Proposed

**Decision.** Agents write most code inside a context I maintain: `CLAUDE.md`, `RULES.md` and skills hold the domain contract, safety rules and gotchas. Plans are reviewed before implementation. I keep decisions about the segment DSL, validation rules, idempotency and retry design. Safety rules are enforced by tests and a hook, not by instructions alone. Each session ends with a four-line entry in `ai-sessions/INDEX.md`: delegated, kept, where the agent was wrong, how I caught it.

**Alternatives.** Free-form prompting without shared context (more rework, inconsistent decisions).

**Consequences.** Slower first hour, fewer reversals later. Evidence for every claim lives in test output and the session records.

<!-- Add new ADRs below. Use the next number. -->

## ADR-013: Strict rolling-window delivery admission and final consent check
Status: Proposed

**Context.** A burst-capable token bucket does not guarantee at most 100 calls in every rolling second. Checking consent before a limiter wait can use stale consent.

**Decision.** Supersede ADR-008 with a context-aware paced limiter per channel, with no accumulated credit and at least 10 ms between provider HTTP call starts. Count retries too. Serialize admission, the final consent lookup and call start per channel; base the next admission on the actual previous call start, so a delayed lookup cannot create a burst. The hard bound is 100 starts in any half-open interval [t, t+1s). One worker replica owns each channel. The mock provider independently enforces the same rolling-window bound.

Immediately after capacity is available, query consent and then start HTTP without another queued wait. Missing/revoked consent skips the delivery; lookup errors fail closed and use bounded retries. No database transaction spans HTTP.

**Alternatives.** Burst-capable token bucket (violates the strict bound); a shared limiter (needed when multiple workers own the same channel).

**Consequences.** Serial per-channel dispatch may achieve less than 100 calls per second when the provider is slow. This is acceptable for the first slice; measure before adding concurrency. The check and an external HTTP request cannot be atomic: revocation after the final check can race with dispatch. Cancellation needs the same final dispatch gate. Multiple replicas per channel are unsupported until shared admission exists.

**Verification.** Fake-clock rolling-window tests cover startup, idle periods, retries, concurrent callers and delayed consent queries. Revoke consent during the limiter wait and assert zero provider calls. Fail the consent query and assert zero provider calls.

## ADR-014: Conservative rendering bounds at approval
Status: Proposed

**Context.** Both allowed variables can expand, and audience data can change after preview.

**Decision.** Compute maximum rune lengths for both `first_name` and `city` from the consent-filtered approval snapshot. Apply both maxima simultaneously to every occurrence in each message field and enforce all field limits. Reject unknown/malformed placeholders and empty audiences. Approval validates and freezes the rendered delivery payloads in the same transaction as enqueueing; retry sends reuse them. Any human edit requires validation again.

**Alternatives.** Measure only first names (unsafe); validate every rendered recipient separately (less conservative, more work).

**Consequences.** Separate maxima can reject a template that fits every actual recipient. If the approval snapshot differs from the preview in membership or content, require human approval of an updated preview. Frozen payloads are personal data: keep them out of prompts, logs and session exports. Consent is still rechecked at dispatch.

**Verification.** Cover maxima from different recipients, repeated variables, Unicode rune counting, every field limit and profile changes between preview and approval.

## ADR-015: Backend foundation and single-instance startup migrations
Status: Proposed

**Context.** The seven-day case study needs a working foundation without a migration framework. The assignment is authoritative for product requirements; the required launch command is plain `docker compose up`.

**Decision.** Use Go net/http and database/sql with the jackc-maintained pgx v5.11.0 driver, verified on pkg.go.dev and its upstream repository before installation. Pin module dependencies in go.mod/go.sum and official container images by digest. Use `/health` for process liveness and `/ready` for a context-bounded PostgreSQL ping. These are canonical instead of the earlier `/healthz` and `/readyz` names in the delivery skill; queue readiness and metrics are later-slice work.

Apply trusted, embedded, numbered SQL files in lexical order in one startup transaction. Store filenames in public.schema_migrations. Failure rolls back and prevents HTTP startup. Previously applied files are immutable by convention; append new files. Run only one API instance until migration coordination is added. No automatic downgrade, checksum framework or distributed runner.

**Alternatives.** PostgreSQL entrypoint initialization scripts (do not apply new migrations to existing volumes); a third-party migration framework (unnecessary dependency here); checksum and coordination framework (outside current scope).

**Consequences.** Pools have 10 open/5 idle connections. Database operations have deadlines; errors omit raw driver details to avoid leaking credentials. The scratch runtime contains only the API binary; current Compose database traffic is local without TLS. Foundation logs have no campaign or delivery identifiers because those entities do not exist yet. Later slices retain the PostgreSQL queue decision in ADR-006.

**Verification.** Endpoint/config tests, safety checks, PostgreSQL migration apply/repeat/rollback tests, Docker startup and database outage/recovery probes. See README for actual results. Keep this record Proposed until required checks pass.

## ADR-016: Draft campaigns and consent-aware segment previews
Status: Superseded by ADR-022

Only the draft-only editing/preview and last-completed-save policies are superseded; other decisions remain applicable.

**Context.** Slice 2 must exercise the validated audience flow before LLM generation or delivery is implemented. Missing consent must exclude recipients, and repeated cart/order rows must not inflate counts.

**Decision.** Add relational user/consent/cart/order data, draft campaigns with JSONB segments and canonical channel arrays, and a versioned seed marker through migration 002. Parse segment JSON strictly into immutable validated types. Exact field spelling, duplicate-key rejection, input bounds and parameter-only values protect the query boundary; consent is added by the compiler. Domain skill holds the canonical DSL limits and time-window semantics.

The API accepts manual segments for now, persists only validated segments and exposes aggregate preview results without contacts or recipient rows. Each preview captures one UTC time and uses a repeatable-read transaction across selected channels. It returns distinct eligible recipients, per-channel counts and independent maximum first-name/city rune lengths. It is not an approval snapshot. Segment changes are restricted to draft state and updates lock the campaign row.

Seed version 1 inserts 5000 deterministic fake profiles and events in one transaction. Compose enables it explicitly. Record the first reference time, preserve later profile/consent changes, and never reseed on restart. Phone identifiers are deliberately non-dialable instead of claiming a reserved Turkish range. Promote the existing pinned golang.org/x/text v0.29.0 to a direct dependency; its cached registry metadata identifies the Go text repository and exact tag. City aliases and normalization are specified in the domain skill.

**Alternatives.** SQL emitted by a model (forbidden); unfiltered base-audience counts (risk of consent bypass); joins over cart events (duplicate recipients); recomputing seed data on startup (destroys revocations); dialable fake numbers (may identify real people).

**Consequences.** Preview loads matching recipients into memory to compute distinct counts and bounds, appropriate for the 5000-user case study; aggregate SQL is the scaling path. Seeding and migrations still assume one API instance. Development seed data ages over time. Real Claude contracts, messages, approval, delivery queue and metrics remain later work; ADR-006 is unchanged.

**Verification.** Unit and HTTP validation tests, TestLLMOutputNeverReachesSQL, PostgreSQL consent/time/deduplication cases, single-channel and empty previews, immutable non-draft segments, migration upgrade and rollback, seed repeat/rollback, live API aggregate checks and restart all passed. The changed domain skill passed the bundled validator using pinned PyYAML 6.0.3 in temporary tooling storage (package and maintainers verified on PyPI before installation). Actual results are recorded in README.

## ADR-017: Separate segment ingress and document limits
Status: Accepted

**Context.** An accepted compact request can exceed 32768 bytes after Go HTML escaping or PostgreSQL JSONB whitespace formatting. Applying the ingress limit again breaks save, read and preview.

**Decision.** Keep HTTP input capped at 32768 bytes. Revalidate canonical and stored documents through the same strict DSL parser with a separate 262144-byte bound. The DSL permits at most 160 city strings of 120 runes; six-byte JSON escapes plus fixed structure remain below this bound. Preserve duplicate-key, exact-field, UTF-8, nesting, type, condition, string and operator checks. No new dependencies, schema or endpoint changes.

**Trade-off.** Internal parsing accepts larger encodings of valid documents while remaining bounded. Removing validation or raising the public request limit would weaken the contract unnecessarily.

**Verification.** Unit, HTTP, PostgreSQL and maintained Postman regressions; actual execution results are recorded in README.

## ADR-018: Mock segment generation through a validated provider boundary
Status: Superseded by ADR-022

Only the draft-only editing/preview and last-completed-save policies are superseded; other decisions remain applicable.

**Context.** Slice 3A needs a key-free end-to-end brief-to-segment path before adding the real Claude SDK, copy generation or streaming. The existing DSL and draft save operation already enforce validation, consent-safe compilation and draft state.

**Decision.** Add bodyless POST `/campaigns/{id}/generate-segment`. Load the stored brief, call the fixture-backed mock through a replaceable provider interface, validate tool input using the existing strict parser, and save through PutSegment. Version the prompt and tool schema. Allow two validation repairs and a separate one-time retry for transient/rate-limited transport errors per attempt, all under one generation deadline. Public requests cannot select scenarios. Fail unsupported provider configuration at startup; real-provider construction remains forbidden in tests.

Record one metadata-only llm_calls row per provider attempt, including failures. The audit write uses a separate two-second cleanup deadline after cancellation; audit failure prevents saving. A successful call record describes provider validation, not a guarantee that the subsequent campaign save succeeds. Migration 003 is additive; previous migrations are unchanged.

**Alternatives.** Implement heuristic natural-language parsing (misrepresents a mock as intelligence); add the Claude SDK now (outside 3A); expose fault scenarios in HTTP input (test controls would become product API); hold a database transaction across generation (long locks).

**Consequences.** The happy fixture is deliberately independent of brief wording. Repeated generations produce the same segment but create new audit rows. Generation does not advance campaign state or trigger sending. Draft state is rechecked at save time; concurrent draft segment edits use last completed save for this case study. Audit storage loss fails closed. Real Claude, content fixtures and SSE remain later work; PostgreSQL queue ADR-006 is unchanged.

**Verification.** Unit/HTTP and PostgreSQL audit/persistence tests passed, including cancellation recording and audit-outage failure. Compose rebuilt and applied migration 003. Local Postman passed 40 requests and 104 assertions. A deliberate validation-bypass mutation made the output-validation test fail; source was restored before final checks. Results are recorded in README.

## ADR-019: Validated message previews over bounded SSE
Status: Accepted

**Context.** Slice 3B needs mock SMS/email generation that appears incrementally while remaining untrusted until validation. The prior segment runner already supplies repair, retry, timeout and metadata auditing. Message persistence and approval are outside this approved slice.

**Decision.** Add bodyless POST `/campaigns/{id}/generate-messages` for drafts with segments. Use consent-filtered aggregate bounds, versioned prompt/schema and a replaceable streaming message provider. Share bounded call execution with ExtractSegment. Strictly validate complete selected-channel JSON, allowlisted placeholders and rendered rune limits using both independent maxima together. Zero-count channels use database schema bounds; empty audiences can preview but cannot be approved later.

Emit attempt and provisional delta events while the mock produces eight-rune chunks. Every attempt (including a transport retry) resets the client's prior display. Emit validated only after complete domain validation and successful audit recording; otherwise emit a safe terminal error. JSON preflight errors retain normal HTTP statuses; after starting SSE, callers must inspect terminal events. Bound display chunks/total bytes, generation deadlines and socket writes. No content or campaign state is persisted. The final payload includes the preview used for validation, not an approval snapshot.

Migration 004 extends the audit call-name allowlist; previous migrations are immutable. Keep segment and message mock scenarios separate. Cancellation/write failure stops generation; audit cleanup remains bounded and metadata-only.

**Alternatives.** Buffer all output until validation (does not exercise streaming); persist provisional text (unsafe); parse partial forced-tool JSON now (real SDK spike belongs to later Claude integration); introduce WebSockets or a broker (unnecessary for one-way finite streams).

**Consequences.** Deltas may be invalid and must be shown as plain text only. Clients use streaming fetch for POST and treat incomplete streams as failure. No resume/history. Mock content is deterministic, not real brief interpretation. Approval must revalidate current audience/content, and SMS encoding warnings, editing and saved messages are deferred. Real Claude partial-tool decoding is not verified by this mock. PostgreSQL queue ADR-006 remains unchanged.

**Verification.** Domain, LLM and real-socket HTTP tests passed, including flushing before completion, disconnect cancellation, repair resets and no validated event on failure. PostgreSQL migration/audit tests passed. Local Postman passed 49 requests and 127 assertions. Deliberately removing city contributions or disabling flushing made the relevant tests fail; both mutations were restored. Actual evidence and sandbox limitations are recorded in README.

## ADR-020: Official Claude SDK behind the existing validated providers
Status: Accepted

**Context.** Slice 3C replaces mock-only selection with a real Claude adapter while preserving the 3A/3B validators, retry/repair budgets, SSE interface and metadata audit. Secrets must stay out of files/logs; tests must not reach Anthropic.

**Decision.** Pin anthropics/anthropic-sdk-go v1.82.0, verified on the official Go registry against its upstream tag/commit. Only internal/llm imports the SDK; implement both provider interfaces. Production construction explicitly reads model/key from the provided environment accessor, refuses APP_ENV=test before credential lookup, disables SDK environment/profile defaults and tracing, fixes the HTTPS endpoint and disallows redirects. Mock mode remains default. Copy the trusted builder CA bundle into scratch for HTTPS.

Force exactly one named tool with the existing JSON Schema and token bound; disable SDK retries so the existing runner owns retry/repair counts. Reject incomplete/max-token, wrong/missing/multiple tool responses and streams without terminal message_stop. Preserve raw tool JSON through final domain validation. Map provider errors to safe codes, retaining observed token counts in metadata only. Protocol-invalid responses enter the bounded validation repair path; refusals are terminal. No model output becomes SQL or executable code.

Decode provisional string prefixes from bounded input_json_delta data, including escaped text and split surrogate pairs, and reuse the existing SSE attempt/delta/validated/error protocol. Keep the full document cap at 32768 bytes, upstream response cap at 1 MiB, stream-event cap at 8192 and block cap at 16. Re-read the bounded JSON prefix per fragment using encoding/json structure plus a small incomplete-string decoder; this trades CPU for a smaller case-study implementation. Context cancellation and HTTP write failures still stop generation. Real model traffic is opt-in runtime behavior, never part of automated verification.

**Alternatives.** Handwritten HTTP client (violates official-SDK decision); free-text JSON extraction (breaks forced-tool contract); automatic SDK retries layered on runner retries (multiplies cost/calls); buffer until whole fields/messages finish (loses the incremental display behavior); permissive external endpoint overrides (risk of credential leakage).

**Consequences.** SDK dependencies increase module size, but the selected SDK root imports no delivery/cloud-provider SDK. Live model/account availability and performance remain unverified without an explicitly authorized manual smoke test. Local Go HTTP fixtures test the actual SDK and provider path, including failures, rather than stubbing it. Public endpoints, schemas, message preview-only behavior and PostgreSQL queue ADR-006 are unchanged. Do not weaken RULES.md or read .env.

**Verification.** make check, module verification, local SDK HTTP/decoder tests, PostgreSQL regressions and Docker CA-bundle build passed. Mock/test Postman passed 49 requests and 131 assertions with exit 0. Removing the terminal-stop guard made the missing-stop regression fail; source was restored. No real Claude request was made. Actual evidence and limitations are recorded in README.


## ADR-021: Compatible forced-tool model configuration
Status: Accepted

**Context.** Slice 3C examples selected claude-sonnet-5-5 while the adapter forces a named tool. Anthropic documents that Sonnet 5.5 rejects forced tool choice; synthetic fixture-model protocol tests cannot detect model capability mismatches.

**Decision.** Keep forced tools and the provider contract. Change the Compose/template example to claude-sonnet-4-6, whose documented thinking-off configuration supports forced tools. Reject known incompatible Sonnet/Opus 5.5 and Fable/Mythos 5.1 identifiers before looking up credentials. Mock remains the default; no SDK dependency change or live call.

**Alternatives.** Change to auto tool choice/structured outputs (changes the output contract); retain a known-incompatible example (runtime failure).

**Consequences.** This is a documented capability correction, not proof of account/model availability or real API acceptance. Other model identifiers still need explicit compatibility review. Sources: [Sonnet 5.5 changes](https://platform.claude.com/docs/en/models/sonnet-5-5/whats-new-sonnet-5-5), [thinking/tool compatibility](https://platform.claude.com/docs/en/build-with-claude/thinking).

**Verification.** TestClaudeIncompatibleModelRejectedBeforeCredentials and TestClaudeConstructorGuard pass locally without network calls.

## ADR-022: Explicit message saves with campaign-wide revisions
Status: Accepted

**Context.** The case study requires streamed suggestions followed by human preview/editing. Persisting late generation results implicitly could overwrite human edits. Previously draft-only segment writes used last-completed-save semantics (ADR-016/018), and message generation read metadata and audience separately.

**Decision.** Keep generate-messages preview-only. Add explicit complete-set GET/PUT messages, reusing the existing validator against a fresh consent-filtered snapshot in the save transaction. Require a strong campaign revision token on every campaign mutation, including generated segments. Migrations 005/006 introduce monotonic revisions, per-channel template rows and recorded validation metadata; prior migrations stay immutable. Lock the campaign and compare the expected revision before mutation. Increment once per accepted write; reject stale revisions or serialization conflicts without partial writes. No full content history or unverifiable generated/human provenance.

Allow editing and generation in draft/content_ready with a segment. Segment writes retain message text but clear validation metadata and return draft. Valid full-set message saves set content_ready. A repeatable-read GenerationInput captures campaign and audience together, releases its transaction, and returns base_revision with final SSE validation. No locks span LLM calls or SSE. Read templates and metadata from one snapshot. Saving a candidate always uses its captured revision, never an automatically refreshed token.

**Alternatives.** Automatic persistence at stream completion (turns previews into writes and creates commit/connection ambiguity); per-field PATCH or per-channel versions (extra partial-state coordination); candidate jobs/history/tokens (unnecessary for this increment).

**Consequences.** One extra save request gives explicit human selection and one persistence path for generated and edited text. Full-set replacement, campaign revision/status and validation metadata commit atomically. Database shape/readiness constraints supplement domain validation; raw template length ceilings allow placeholder expansion. Profiles and consents are mutable outside campaign revisions, so stored validation is historical. Future approval must still revalidate current membership/content and reject empty audiences. No approval, queue, delivery, worker or frontend is introduced. Provider audit success continues to mean validated provider output, not persisted templates. Caller reconciliation is required after a stale write or lost response.

This supersedes ADR-016/018 editing/preview/concurrent-save policies. ADR-019 preview-only persistence behavior is preserved; its validated event alone never establishes saved content readiness.

**Verification.** Unit/HTTP, PostgreSQL upgrade/rollback/current-audience/concurrency tests and local mock/test Postman passed. Removing the missing-revision guard made its regression fail; the guard was restored before final checks. Actual results are recorded in README.
