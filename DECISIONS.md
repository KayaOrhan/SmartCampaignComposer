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
Status: Accepted

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
