# 2026-10-22 persistence approval worker

Source: authentic Codex conversation `01a162af-0617-7792-b228-0842e0b191e9`. This is a Codex export, not a Claude session.

Source SHA-256: `571a2fcea0b04efc50c0e41e9c9d022a19c8d882d72c203c5e0cfc63e29b17b5`.

Format: chronological public user prompts and assistant commentary/final messages. System/developer instructions, reasoning, repeated injected project/environment context, tool calls/results, UI attachment metadata and compaction summaries are omitted. Tool evidence remains in the original assistant reports; historical claims are not fresh verification. Redactions are marked. No dialogue is reconstructed.

## 1. User (2026-10-22T08:16:51.893Z)

We have completed Slices 3A, 3B, and 3C of Smart Campaign Composer.

Before moving forward, I want a thorough technical review of the current implementation and a concrete plan for the next increment.

## Part 1 — Slice 3C Verification Review

Review the actual repository implementation, tests, ADRs, and acceptance criteria.

Clearly distinguish between:

1. **Verified:** Behaviors demonstrated by existing unit, integration, Docker, Postman, and local Anthropic protocol tests.
2. **Not yet verified:** Behaviors requiring real Anthropic API access.
3. **Potential issues:** Architectural weaknesses, missing edge cases, incorrect assumptions, or integration risks.

Pay particular attention to:

- Anthropic SDK integration and provider interfaces.
- Model compatibility with forced tool use and eager input streaming.
- Structured output validation and streaming JSON decoding.
- Retry/repair behavior, including overlapping retry mechanisms.
- Timeout propagation, cancellation, and SSE connection handling.
- Error sanitization, logging, and audit records.
- Prompt injection boundaries and token/output limits.
- Differences between mock fixtures and actual Claude responses.

Do not claim real API compatibility based solely on mocked protocol tests.

For every finding, reference the relevant source files and functions. Classify issues as blocking, important, or optional.

## Part 2 — Plan the Next Increment: Message Persistence and Human Editing

The current `generate-messages` endpoint produces validated streaming previews but does not persist messages.

Design the next independently reviewable increment that introduces:

- Persistent validated SMS/email message templates.
- Retrieval of saved campaign messages.
- Human editing of generated messages.
- Revalidation of every edited message using existing domain rules.
- Campaign revision tracking and optimistic concurrency protection.
- Clear draft/content-ready state transitions.
- Safe handling of concurrent generation, editing, and stale results.
- Appropriate PostgreSQL migrations and transactional boundaries.

Reuse existing domain validators, provider interfaces, repository patterns, and HTTP conventions.

Avoid introducing unnecessary abstractions or overengineering.

Explicitly define the lifecycle of a generated message:

**Generate → Validate → Persist → Retrieve/Edit → Revalidate → Ready for Approval**

Clarify whether the existing streaming endpoint should persist automatically or whether persistence should be an explicit separate operation. Compare both approaches and recommend one based on the case-study requirements.

## Part 3 — Deliverables

Provide:

1. A proposed runtime flow from HTTP request to PostgreSQL persistence.
2. Exact API endpoint contracts, including request/response examples and error handling.
3. Database schema changes and campaign state transitions.
4. File-by-file implementation plan.
5. Unit, integration, concurrency, and Postman test coverage.
6. Risks, trade-offs, and any required ADR updates.
7. A recommended implementation sequence broken into small, independently testable steps.

Respect `AGENTS.md`, `CLAUDE.md`, `RULES.md`, `DECISIONS.md`, and `IMPLEMENTATION_PLAN.md`.

**Important constraints:**

- Do not implement anything yet.
- Do not modify files or create commits.
- Do not introduce approval, enqueueing, workers, delivery, or frontend functionality in this increment.
- Preserve the existing mock-first development workflow.
- Keep real Anthropic API calls out of automated tests.
- Do not weaken validation or existing security boundaries.
- Avoid speculative requirements that are not justified by the case study.

Finally, give me your recommended next step and explicitly identify any decisions requiring my approval before implementation


## 2. Assistant (2026-10-22T08:16:55.317Z)

I’ll review the repository guidance, Slice 3C code and test evidence, then propose a persistence and editing increment. I won’t modify files or make real Anthropic API calls.


## 3. Assistant (2026-10-22T08:17:08.281Z)

The working tree contains uncommitted Slice 3C changes, so I’ll review those as part of the current implementation. I’m applying the campaign-domain, LLM-contract, verification, and Ponytail skills to trace the behavior and keep the proposed increment minimal.


## 4. Assistant (2026-10-22T08:17:48.517Z)

I found a blocking live-provider mismatch: Compose defaults to `claude-sonnet-5-5`, but Anthropic’s current documentation says that model rejects forced tool use. The adapter always forces a named tool. The local fixtures use `fixture-model`, so they cannot catch this.

I’m also checking concurrency around campaign reads and generation, where the persistence increment will need stronger guarantees.


## 5. Assistant (2026-10-22T08:19:50.546Z)

The uncached LLM and HTTP suites passed, and `make check` passed after allowing local fixture connections. PostgreSQL verification is running.

For the next increment, I recommend keeping generation preview-only and adding an explicit save/edit operation. A campaign revision would protect that write from stale generations and concurrent edits, while the save transaction would revalidate against a fresh audience preview.


## 6. Assistant (2026-10-22T08:20:09.114Z)

The PostgreSQL suite and Docker API build passed. I’ll now run the maintained Postman collection against the local mock/test API.

Two current behaviors need to change before persistence is safe: segment writes use last-completed-save semantics, and message generation reads the campaign and audience in separate snapshots. The plan will add revision checks and read generation inputs from one consistent snapshot.


## 7. Assistant (2026-10-22T08:21:10.047Z)

Postman passed: **49 requests, 131 assertions, zero failures, exit 0**, against the local mock/test API.

The remaining findings concern live model compatibility, gaps in protocol-error coverage, retry cost and timing, and concurrency guarantees. The report will separate demonstrated behavior from what still requires an authorized real-API smoke test.


## 8. Assistant (2026-10-22T08:24:20.518Z)

**Slice 3C has passing local verification, but its default live-model configuration has a blocking compatibility problem.** The next increment can proceed independently in mock mode once its persistence and concurrency contract is approved.

No repository files were modified, staged, or committed. Verification rebuilt local Docker images, reset the guarded `scc_test` database, and created one synthetic campaign through Postman.

**1. Verification evidence**

I reviewed the working tree, including uncommitted Slice 3C files, project instructions, case study, ADRs, implementation plan, validators, handlers, storage code, fixtures, and tests.

Checks executed during this review:

| Check | Result |
|---|---|
| `make check` | Passed after granting local socket access |
| Uncached LLM and HTTP suites | Passed with `-count=1` |
| Docker PostgreSQL integration suite | Passed |
| Docker API build/start in mock/test mode | Passed |
| Local Postman collection | **49 requests, 131 assertions, zero failures, exit 0** |
| `go mod verify` | `all modules verified` |
| `git diff --check` | Passed |

The initial sandbox runs failed because loopback/Docker access was denied; the unrestricted reruns passed. No tests were skipped or altered.

Relevant `make check` output:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

These results establish local behavior, **not real Anthropic compatibility**.

**Verified behavior**

| Area | Demonstrated behavior and evidence |
|---|---|
| SDK integration | Actual pinned SDK serialization and HTTP consumption against loopback servers; forced tool name, schema, token budget, usage metadata. `TestClaudeSDKSegmentAndStream` in [anthropic_test.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/anthropic_test.go:104). |
| Provider boundary | Both adapters feed the existing validation runners; callers receive validated domain objects or safe typed errors. `SegmentGenerator.ExtractSegment`, `MessageGenerator.GenerateMessages`, and `runCall` in [segment.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/segment.go:82) and [messages.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/messages.go). |
| Structured validation | Strict JSON parsing, duplicate/unknown-field rejection, selected channels, placeholders, rune limits, and both independent audience maxima. `ValidateMessages` and `validateTemplate` in [campaign/messages.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/campaign/messages.go:32). |
| Streaming decoding | Synthetic fragments exercise quotes, escapes, byte boundaries, Unicode, split surrogate pairs, duplicate display fields, and document bounds. `TestToolJSONFragments` and `TestToolJSONProvisionalPrefixAndBounds` in [tool_stream_test.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/tool_stream_test.go). |
| Failure handling | Local fixtures cover HTTP 400/401/403/429/500/529, stream overload, refusals, missing/wrong/multiple tools, truncation, token exhaustion, and missing terminal stop. [anthropic_test.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/anthropic_test.go:167). |
| Retries and repairs | SDK retries are disabled; runner retry counts and repair/refusal behavior are asserted locally. `newAnthropic`, `runCall`, `TestClaudeHTTPFailuresAndRetry`, and `TestClaudeHTTPRepairAndTransientRecovery`. |
| Cancellation and SSE | Real local sockets demonstrate flushing before completion and disconnect cancellation; SDK fixtures demonstrate upstream cancellation. [streaming_test.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/httpapi/streaming_test.go:191) and `TestClaudeSDKDeadlineAndCancel`. |
| Audit persistence | PostgreSQL tests exercise per-attempt metadata records, timeout outcomes, audit-outage failure, and preview-only state. `TestGeneratedSegmentPersistenceAndAudit` and `TestMessageGenerationAuditAndPreviewOnly` in [domain_integration_test.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/store/domain_integration_test.go:187). |
| HTTP regression | Postman checks finite SSE framing, reconstructed fields, final validation, safe envelopes, and unchanged draft state. It buffers responses and therefore does **not** prove incremental flushing. |

**Not yet verified**

The following require an explicitly authorized manual real-API exercise:

- Account access and availability of the selected model.
- Acceptance of the actual model/tool/schema combination.
- Public TLS connectivity from the scratch container.
- Real fragment patterns, latency, token usage, and repair success.
- Whether the current 10-second default adequately accommodates real generation.
- Brief interpretation and copy quality.

Synthetic injection tests prove that malicious output cannot bypass the tested validators or become executable SQL. They do not demonstrate that Claude reliably resists injected instructions.

**2. Technical findings**

Severity meanings: **blocking** prevents the intended behavior; **important** should be addressed before relying on the affected behavior; **optional** is bounded improvement work.

**Blocking — default model rejects the adapter’s forced tool configuration**

[Compose](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/docker-compose.yml:26) defaults to `claude-sonnet-5-5`. `Anthropic.params` always sends named forced tool choice in [anthropic.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/anthropic.go:80).

Anthropic’s current documentation states that Sonnet 5.5 rejects `tool_choice` types `tool` and `any` with HTTP 400. [Official model documentation](https://platform.claude.com/docs/en/models/sonnet-5-5/whats-new-sonnet-5-5).

Consequences:

- Opting into Anthropic with the Compose default should fail both generation paths.
- `safeSDKError` sanitizes that rejection into `llm_unavailable`, obscuring the configuration cause.
- Local tests use `fixture-model`, so they cannot establish model compatibility.

**Recommendation:** preserve the current forced-tool design and choose a documented compatible model explicitly. Moving to automatic tool choice or structured outputs changes ADR-003/020 and should be a separate approved decision.

This blocks a live-provider readiness claim; it does not block mock-first persistence work.

**Important — campaign revision protection is absent**

`Campaigns.PutSegment` locks the row and checks draft state, but does not compare a revision. A slow generated segment can overwrite a newer manual segment. ADR-018 explicitly accepts last-completed-save behavior.

Sources: [store/campaign.go — PutSegment](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/store/campaign.go:67), [generation.go — registerGeneration](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/httpapi/generation.go), ADR-018.

This becomes unsafe once saved messages depend on the segment. Revision protection must cover **all campaign mutations**, including generated segments, rather than only the new message endpoint.

**Important — generation inputs are read from separate snapshots**

`registerMessageGeneration` calls `Get`, then `Preview`. `Preview` independently rereads the campaign in its own repeatable-read transaction. A concurrent segment change can therefore make generation use campaign metadata from one revision and audience bounds from another.

Sources: [streaming.go](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/httpapi/streaming.go:21), [audience.go — Preview](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/store/audience.go:13).

Currently this produces ephemeral previews. Persistence must use a coherent generation snapshot and revalidate at save time.

**Important — malformed SSE event JSON does not follow the documented protocol-repair path**

`Anthropic.GenerateMessages` maps `stream.Err()` through `safeSDKError`. SDK event decoding errors therefore become generic unavailable errors, whereas adapter-detected protocol failures become `errProviderOutput` and enter repair.

Sources: [anthropic.go — GenerateMessages and safeSDKError](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/anthropic.go:101), [segment.go — runCall](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/segment.go:103).

ADR-020 broadly says protocol-invalid responses enter repair. Existing tests cover missing/truncated events and malformed tool input, but not malformed JSON in the SSE event envelope.

**Recommendation:** add a local regression and explicitly classify decoder failures versus transport failures. Continue discarding raw error details.

**Important — the complete retry budget can reach six provider calls**

There is no accidental SDK retry multiplication: `option.WithMaxRetries(0)` prevents that. However, `runCall` allows one transient retry within each of three validation attempts:

```text
3 validation attempts × 2 transport attempts = up to 6 calls
```

Sources: `newAnthropic` and `runCall`.

That is bounded and consistent with ADR-018, but mixed transient/invalid sequences are not directly covered by the current retry tests. All calls share one deadline, so repair opportunities can disappear after a slow first attempt.

The fixed 100 ms retry also ignores provider retry timing. This is a reliability/cost limitation, not an unbounded-retry defect.

**Recommendation:** test the mixed six-call ceiling and deadline behavior; document the total budget. Consider bounded provider-directed backoff separately.

**Important — fixtures underrepresent model-dependent behavior**

`MessageMock.GenerateMessages` removes unselected channels before returning fixture content. The real adapter does not: it depends on the prompt, and final validation rejects extras. The message schema itself permits either channel rather than expressing the requested channel set.

Sources: [messages.go — MessageMock.GenerateMessages](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/messages.go:108), [messages-v1.schema.json](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/prompts/messages-v1.schema.json), `Anthropic.GenerateMessages`.

The final validator remains protective. However, passing single-channel mock tests says little about real single-channel generation reliability.

**Recommendation:** add local SDK fixtures for single-channel responses and incorrect extra channels. A channel-specific schema is an optional later improvement.

**Optional — parser and protocol coverage can be broadened**

`toolJSON.feed` reparses the accumulated prefix for every fragment. Work is bounded, but numerous tiny fragments amplify CPU cost. ADR-020 acknowledges this trade-off.

`Anthropic.GenerateMessages` also does not fully validate the correspondence between active block type and every delta type. Unknown events are ignored, and its event count covers SDK-returned events rather than raw ping/unknown frames.

Sources: [tool_stream.go — feed](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/tool_stream.go:32), [anthropic.go — GenerateMessages](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/llm/anthropic.go:152).

Keep the current implementation unless profiling or adversarial fixtures justify replacement. Add reordered-field, wrong-delta-type, and fragment-volume cases before claiming comprehensive protocol hardening.

**Optional — timeout does not guarantee a terminal SSE error**

The handler sets the socket write deadline to its request deadline. Once that expires, writing the terminal error may fail. Audit cleanup can continue for up to two seconds through `context.WithoutCancel`.

Sources: [streaming.go — SetWriteDeadline/writeEvent](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/httpapi/streaming.go:63), `runCall`.

This is acceptable if clients treat every stream without a terminal success as failure, as ADR-019 requires. Tests should distinguish cancellation propagation from guaranteed delivery of an error event.

**Optional — audit success must remain distinct from content persistence**

The current audit records provider validation success, not campaign save success. They contain neither a generation correlation identifier nor campaign revision. Cancellation is recorded as `llm_timeout`.

Sources: `runCall`, [store/llm.go — RecordLLMCall](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/store/llm.go), ADR-018.

For the next increment, record safe campaign ID/revision metadata around saves. Do not interpret `llm_calls.outcome=success` as evidence that templates were stored.

I found no reason to weaken error sanitization, prompt escaping, immutable validated types, or metadata-only audit storage. Prompt compliance still cannot establish semantic correctness, and the 4096-token message budget does not guarantee every permitted 2000-rune email can complete.

**3. Recommended increment: explicit template persistence and editing**

The lifecycle should be:

```text
Generate
  → Validate complete provider output
  → Explicitly save, with revision check and fresh revalidation
  → Retrieve/Edit
  → Revalidate and atomically replace
  → content_ready for a future approval step
```

No approval endpoint or delivery behavior belongs here.

| Approach | Benefit | Cost/risk |
|---|---|---|
| Persist automatically at successful stream completion | Fewer client calls; server knows generated provenance | Generation becomes a write; late completions can overwrite edits without concurrency guards; commit may succeed even if the final SSE event is lost |
| Explicit save after preview | Preserves existing preview behavior; one save path handles generated and edited templates; saving reflects deliberate content selection | One additional request; preview must be retained by the caller until saved; server cannot prove generated provenance from submitted bytes |

**Recommend explicit save.** The case study requires streaming generation followed by human preview/editing; it does not require automatic storage during generation. Explicit save also keeps the existing provider interfaces and audit semantics intact.

Do not add generation jobs, candidate history, signed preview tokens, or provenance infrastructure. Treat every submitted template as untrusted, including a previously validated preview.

**4. Runtime and transaction flow**

For generation:

1. Read campaign, revision, selected channels, and audience aggregates in **one repeatable-read snapshot**.
2. Release the transaction.
3. Generate through the unchanged provider interfaces, retry/repair runner, validators, and audit recorder.
4. Stream provisional deltas.
5. Include the captured campaign revision in the final validated preview.
6. Persist no templates and change no campaign state.

For save/edit:

1. Bound and strictly decode the request.
2. Require the revision the caller edited or generated against.
3. Begin a short, deadline-bound repeatable-read transaction.
4. Lock and read the campaign; check revision and editable state.
5. Compute current consent-filtered audience bounds inside that transaction using one captured UTC `as_of`.
6. Run `campaign.ValidateMessages` on the submitted raw message document.
7. Replace the complete selected-channel template set.
8. Increment campaign revision and store validation metadata; set `content_ready`.
9. Commit, then return the saved representation.

Never hold a transaction while calling Claude or streaming to a client.

Refactor the current audience calculation into a small transaction-scoped helper. Reuse `segment.Validated.Query`, existing validators, `finishTx`, deadlines, and safe error mapping.

**Concurrency rules**

- Every accepted segment or message mutation increments one campaign revision.
- Two writers using the same revision cannot both succeed.
- A slow generated segment cannot overwrite a newer segment or message edit.
- Generations are read-only; several can coexist.
- Saving an old generation after another mutation fails its revision check.
- Do not automatically resubmit stale content using a newly fetched revision. The caller must reconcile it.
- PostgreSQL serialization failures abort the entire operation and return a safe conflict.
- Profile/consent changes do not increment campaign revision. Fresh transactional revalidation handles their effect on lengths; future approval must still recheck its audience snapshot.

Revision tracking here means a monotonic concurrency version, **not a historical content ledger**.

**5. Proposed API contracts**

Use a required `If-Match` campaign revision token for mutations:

```http
If-Match: "campaign-42-r7"
```

Return `revision` in campaign JSON. For message responses, return the same campaign revision explicitly. Treat the token as a campaign concurrency precondition.

**Existing `GET /campaigns/{id}`**

Add:

```json
{
  "id": 42,
  "brief": "Synthetic campaign brief",
  "channels": ["sms", "email"],
  "segment": {
    "version": 1,
    "all": [{"field": "city", "op": "in", "value": ["istanbul"]}]
  },
  "status": "draft",
  "revision": 7,
  "created_at": "2026-10-22T09:00:00Z"
}
```

Return the corresponding revision token in `ETag`.

**Existing `POST /campaigns/{id}/generate-messages`**

Remain bodyless and preview-only. Permit `draft` and `content_ready`, provided a segment exists.

Keep attempt/delta/error semantics. Extend final success:

```text
event: validated
data: {"attempt":1,"base_revision":7,"messages":{"sms":{"body":"Merhaba {{first_name}}!"},"email":{"subject":"Yeni teklif","body":"Merhaba {{first_name}}, teklifimizi inceleyin."}},"audience":{"as_of":"2026-10-22T09:10:00Z","eligible_unique":2,"by_channel":{"sms":{"count":2,"max_first_name_runes":10,"max_city_runes":12},"email":{"count":2,"max_first_name_runes":10,"max_city_runes":12}}}}
```

`validated` means valid against that captured preview; it does not mean saved or approved. Callers must retain `base_revision` with the messages.

**New `GET /campaigns/{id}/messages`**

Return HTTP 200, including when no templates exist:

```json
{
  "campaign_id": 42,
  "revision": 7,
  "status": "draft",
  "messages": null,
  "validation": null
}
```

After a save:

```json
{
  "campaign_id": 42,
  "revision": 8,
  "status": "content_ready",
  "messages": {
    "sms": {"body": "Merhaba {{first_name}}!"},
    "email": {
      "subject": "Yeni teklif",
      "body": "Merhaba {{first_name}}, teklifimizi inceleyin."
    }
  },
  "validation": {
    "revision": 8,
    "audience": {
      "as_of": "2026-10-22T09:11:00Z",
      "eligible_unique": 2,
      "by_channel": {
        "sms": {"count": 2, "max_first_name_runes": 10, "max_city_runes": 12},
        "email": {"count": 2, "max_first_name_runes": 10, "max_city_runes": 12}
      }
    }
  }
}
```

Read campaign and templates from one consistent snapshot. Return stored validation metadata as historical evidence; GET must not silently mutate state or imply a current approval snapshot.

**New `PUT /campaigns/{id}/messages`**

```http
Content-Type: application/json
If-Match: "campaign-42-r7"
```

```json
{
  "messages": {
    "sms": {"body": "Merhaba {{first_name}}!"},
    "email": {
      "subject": "Yeni teklif",
      "body": "Merhaba {{first_name}}, teklifimizi inceleyin."
    }
  }
}
```

- Full replacement of exactly the selected channels.
- Same endpoint for saving generated copy and submitting human edits.
- No client-supplied validation results, audience bounds, status, or trusted provenance.
- HTTP 200 returns the saved representation above.
- Invalid submissions preserve templates, state, validation metadata, and revision.
- No partial field PATCH in this increment.

**Existing segment mutations**

Require `If-Match` on:

- `PUT /campaigns/{id}/segment`
- `POST /campaigns/{id}/generate-segment`

Preserve their existing bodies. Generated segments compare the captured revision again when saving.

Allow segment changes from `draft` and `content_ready`. An accepted segment mutation retains existing template text but clears its validation metadata and returns the campaign to `draft`.

**Errors**

Preserve the current safe envelope:

```json
{"error":{"code":"campaign_revision_conflict","path":"revision"}}
```

| HTTP | Proposed behavior |
|---|---|
| 400 | Malformed/duplicate JSON or malformed revision token |
| 404 | Campaign missing |
| 409 | Invalid state, segment missing, or transactional serialization conflict |
| 412 | Supplied revision is stale |
| 413 | Existing 32768-byte ingress limit exceeded |
| 422 | Existing structural/domain validation errors |
| 428 | Required revision precondition missing |
| 503 | Database unavailable or deadline failure |

For serialization failure, use a distinct `concurrent_update` code. Do not expose SQLSTATE details or raw driver messages.

Existing generation errors retain their current HTTP/SSE conventions. Once SSE starts, HTTP status alone remains insufficient.

**6. Schema and state changes**

Add one immutable migration, provisionally `005_campaign_messages.sql`.

| Table | Changes |
|---|---|
| `campaigns` | `revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)` |
| `campaigns` | Nullable `messages_validated_revision BIGINT` |
| `campaigns` | Nullable `message_validation_preview JSONB`, constrained to an object |
| `campaign_messages` | `campaign_id` FK, `channel`, nullable `subject`, non-null `body`; primary key `(campaign_id, channel)` |

Add database checks for:

- Allowlisted channels.
- SMS has no subject; email has a nonempty subject.
- Nonempty bodies and basic field-length ceilings.
- Validation revision and preview are either both absent or both present.
- `content_ready` requires a segment and validation at the current revision.

Rendered lengths remain enforced by Go because they depend on audience bounds. Exact selected-channel completeness is enforced transactionally; simple CHECK constraints cannot enforce that cross-table invariant.

The domain skill sketches `edited_by_human`. With explicit save, the server cannot reliably determine whether submitted bytes were edited from a generated preview. **Recommend omitting that flag in this increment**, documenting the deviation, and avoiding misleading provenance.

State behavior:

| Operation | Result |
|---|---|
| Create | `draft`, revision 1 |
| Save/change segment | `draft`; revision increments; retained messages require revalidation |
| Generate message preview | No state/revision change |
| Successfully save complete validated messages | `content_ready`; revision increments |
| Successfully edit/revalidate messages | `content_ready`; revision increments |
| Invalid/stale save | No changes |
| Mutation after `approved` or another noneditable state | Rejected |

`content_ready` means complete, saved content validated against its recorded audience preview. It is not approval. Empty audiences may retain valid content, consistent with existing preview behavior; future approval must reject an empty audience.

**7. File-by-file implementation plan**

All paths below are relative to the repository root.

| File | Planned work |
|---|---|
| `migrations/005_campaign_messages.sql` | Revision, template table, validation metadata, constraints |
| `internal/campaign/campaign.go` | Revision and safe conflict errors |
| `internal/campaign/messages.go` | Saved-message representation; reuse existing validation |
| `internal/store/campaign.go` | Scan revision; conditional segment writes; invalidate content validation |
| `internal/store/audience.go` | Reuse aggregation within caller transactions; coherent generation snapshot; editable-state support |
| `internal/store/messages.go` — new | Transactional retrieval, revalidation, and full replacement |
| `internal/httpapi/campaign.go` | Revision precondition parsing and safe error mapping |
| `internal/httpapi/messages.go` — new | GET/PUT messages |
| `internal/httpapi/generation.go` | Conditional generated-segment save |
| `internal/httpapi/streaming.go` | Coherent inputs and `base_revision`; remain preview-only |
| Existing campaign/HTTP/LLM tests | Update interfaces and revision contracts; preserve regressions |
| `internal/store/*integration_test.go` | Migration, transactional, persistence, and concurrent-writer tests |
| `postman/SmartCampaignComposer.postman_collection.json` | Capture revision tokens; add persistence/edit/conflict regressions |
| `postman/README.md` | Updated local contracts and coverage |
| `DECISIONS.md` | New persistence/concurrency ADR and explicit supersessions |
| `IMPLEMENTATION_PLAN.md` | Approved increment and boundaries |
| `README.md` | API examples, evidence, and limitations |
| `.claude/skills/campaign-domain/SKILL.md` | Align revision/state and provenance description |

No new dependency, provider interface, queue, worker, frontend, or approval abstraction is needed.

**8. Required test coverage**

| Layer | Coverage |
|---|---|
| Unit/domain | Existing placeholder/rune/maxima rules; selected-channel completeness; malformed JSON; immutable validated values |
| HTTP | Save/retrieve/edit; absent messages; strict envelope; missing/malformed/stale preconditions; safe error codes; rejected writes preserve state |
| PostgreSQL | Upgrade/repeat/rollback migration; full-set atomic replacement; read consistency; validation metadata; state/revision transitions |
| Concurrency | Two edits at one revision; edit versus segment change; generated segment versus edit; old preview save; serialization conflict |
| Mutable audience | Profile expansion causes fresh save rejection; consent changes affect bounds; empty-channel fallback remains enforced |
| SSE | `base_revision` matches coherent inputs; generation never persists; generation allowed from `content_ready`; existing cancellation/flush tests remain |
| Postman | Generate → save → retrieve → edit → retrieve; validation failures preserve data; stale revision rejection; segment change invalidates readiness while retaining text |

Use synchronization barriers for concurrency tests, rather than sleeps. Assert exactly one successful writer, one revision increment, and no mixed channel set.

Retain mock-first execution and all local SDK fixtures. Real Anthropic calls remain outside automated tests.

**9. ADRs, trade-offs, and implementation sequence**

Add a proposed ADR covering explicit persistence, aggregate revision checks, atomic template replacement, fresh audience validation, and retained-but-invalidated content.

It should explicitly supersede:

- ADR-018’s last-completed segment save policy.
- ADR-016’s draft-only segment editing restriction.
- The draft-only preview restriction where `content_ready` must remain editable.

ADR-019’s preview-only generation decision remains intact. Clarify that its validated event does not itself establish persisted readiness.

Keep the live-model correction separate from persistence. Preserve ADR-014’s future approval revalidation requirement.

Recommended independently testable sequence:

1. **Resolve the live-model configuration decision** in a small separate change; add a regression for known-incompatible configuration.
2. **Introduce revisions and conditional segment writes**, updating existing HTTP and Postman contracts.
3. **Add template storage and GET/PUT**, with transactional revalidation and state transitions.
4. **Integrate coherent generation snapshots and `base_revision`**, retaining preview-only behavior.
5. **Complete concurrency and failure verification**, then update evidence and known gaps.

The main trade-off is one extra save request in exchange for explicit selection and simpler concurrency semantics. The main remaining limitation is that campaign revision does not freeze mutable audience data; save-time validation and later approval-time validation have different responsibilities.

**Recommended next step and approvals**

Approve a **Slice 3D: persistent message templates and human editing** plan before implementation, as required by `AGENTS.md` and `CLAUDE.md`.

The decisions needing your approval are:

1. **Explicit save**, keeping streaming generation preview-only.
2. **Campaign-wide optimistic concurrency**, including the existing segment endpoints and their new required revision preconditions.
3. **Retain template text but invalidate readiness after segment changes**.
4. **Monotonic revisions without content history**, and omission of unverifiable generated/edited provenance.
5. **A separate compatible-model correction**, preserving forced tool use rather than redesigning the provider contract.

A real Anthropic smoke test requires separate explicit authorization. It is not a prerequisite for implementing and verifying this increment in mock mode.

## 9. User (2026-10-22T08:26:24.073Z)

I approve the proposed Slice 3D architecture and all five decisions.

Please proceed incrementally, following AGENTS.md, CLAUDE.md, and RULES.md.

Requirements:

- Keep message generation preview-only, with explicit persistence.
- Implement campaign-wide optimistic concurrency and revision preconditions.
- Preserve existing message text after segment changes, but invalidate readiness and require revalidation.
- Use monotonic revisions without full content history.
- Handle compatible-model configuration as a separate, minimal correction.
- Ensure save-time validation uses current audience bounds.
- Do not implement approval, delivery, workers, or frontend yet.
- Keep mock as the default and do not call the real Anthropic API.

Implement and verify one increment at a time. Do not commit without my approval.

After implementation, explain the new runtime flow, endpoints, state transitions, and how to test them in Postman.


## 10. Assistant (2026-10-22T08:26:27.678Z)

I’ll start with the separate model-configuration correction, verify it, then implement revision protection and message persistence in small steps. Mock mode will remain the default, and I won’t make real Anthropic calls or commit changes.


## 11. Assistant (2026-10-22T08:26:41.201Z)

I’m applying the campaign-domain, LLM-contract, verification, and Ponytail skills. I’ll first correct and test model configuration, then add and verify revisions, and finally add transactional message storage, editing, and Postman coverage. The existing uncommitted Slice 3C work will remain intact.


## 12. Assistant (2026-10-22T08:28:41.239Z)

The model-configuration correction is in place, and its focused tests passed. The examples now use `claude-sonnet-4-6`; known models that reject forced tool use are rejected before credential lookup. This is checked against the documented contract, with no real API call made.


## 13. Assistant (2026-10-22T08:33:17.414Z)

Revision checks are implemented, and the HTTP suite passed. I’m verifying the migration and competing segment writes in PostgreSQL before moving on to message storage. Segment updates and generated segments now require a revision token; a generation that finishes late will be rejected if another write has advanced the campaign.


## 14. Assistant (2026-10-22T08:35:53.229Z)

The revision increment passed PostgreSQL and Postman verification. Message storage and editing are now implemented, and `make check` passed.

I’m adding tests for the main failure cases: saves after audience bounds change, stale preview saves, competing edits, segment changes during editing, and rollback if only part of a channel set is written.


## 15. Assistant (2026-10-22T08:41:31.889Z)

The expanded Postman lifecycle passed: **73 requests, 173 assertions, zero failures**. It covers explicit save, retrieval, human edits, stale revisions, validation rejection, and retained text after segment changes.

The PostgreSQL tests also passed for competing edits, edits racing segment changes, and rollback after a partial replacement attempt. I’m finishing migration and cancellation checks, documentation, and a review of the actual diff.


## 16. Assistant (2026-10-22T08:46:50.277Z)

The final Postman collection passed: **74 requests, 176 assertions, zero failures**. PostgreSQL checks passed for upgrades from Slice 3C, single-channel saves, cancellation, and templates whose placeholders are longer than their rendered output.

I also temporarily removed the missing-revision rejection and confirmed that `TestRevisionPreconditions` failed. The guard is restored, and `make check` is green.


## 17. Assistant (2026-10-22T08:49:49.062Z)

Implemented Slice 3D incrementally and verified it. **No commits or real Anthropic calls were made.** Existing Slice 3C changes remain intact.

The runtime flow is now:

**Generate preview → Validate → Explicitly save → Retrieve/Edit → Revalidate → `content_ready`**

Generation reads campaign metadata and audience bounds from one consistent snapshot, then closes the transaction before calling the provider. Its final SSE event includes `base_revision`; generation never persists templates.

Saving locks the campaign, checks its revision, computes fresh consent-filtered audience bounds, and runs the existing message validators. Templates, validation metadata, state, and revision commit atomically.

| Endpoint | Behavior |
|---|---|
| `POST /campaigns/{id}/generate-messages` | Preview-only SSE, including `base_revision` |
| `GET /campaigns/{id}/messages` | Retrieve saved templates and validation metadata |
| `PUT /campaigns/{id}/messages` | Save or edit the complete selected-channel set |
| `PUT /campaigns/{id}/segment` | Change segment and invalidate content readiness |
| `POST /campaigns/{id}/generate-segment` | Generate and conditionally save a segment |

All three write endpoints require:

```http
If-Match: "campaign-42-r7"
```

Every successful write increments the campaign revision once. Missing preconditions return **428**, malformed tokens **400**, stale revisions **412**, and transactional concurrency conflicts **409**.

A message save uses this envelope:

```json
{
  "messages": {
    "sms": {"body": "Merhaba {{first_name}}!"},
    "email": {
      "subject": "Updated offer",
      "body": "An offer for {{city}}."
    }
  }
}
```

State transitions are:

- Valid message save/edit → `content_ready`.
- Segment change → `draft`, retaining message text and clearing validation metadata.
- Invalid or stale write → no changes.

`content_ready` records successful content validation; it does not approve or freeze the audience.

Migrations **005 and 006** add revisions, message rows, and validation metadata. The separate model correction changes the example model to `claude-sonnet-4-6` and rejects known incompatible forced-tool models before credential lookup. Live compatibility remains unverified.

Verification passed:

- `make check`, including:
  ```text
  repository safety and formatting checks passed
  go vet -mod=readonly ./...
  go test -mod=readonly ./...
  ```
- Docker PostgreSQL migration, lifecycle, fresh-bound, rollback, cancellation, and competing-writer tests.
- Postman: **74 requests, 176 assertions, zero failures, exit 0**.
- Module verification and diff checks.
- A deliberately removed revision guard caused its regression test to fail; the guard was restored.

The race detector remains unavailable because of the existing missing CGO compiler; PostgreSQL concurrency tests did execute.

To test in Postman, import the maintained collection and run it **in order** against the local mock/test API. It captures IDs and revision tokens automatically. The new 3D folder exercises generation, explicit save, retrieval, editing, stale-write rejection, segment invalidation, and revalidation.

Detailed examples and startup commands are in [the Postman walkthrough](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/postman/README.md:47) and [runtime documentation](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/README.md:54).

## 18. User (2026-10-22T08:54:46.663Z)

Now return to the original case-study requirements, IMPLEMENTATION_PLAN.md, CLAUDE.md, AGENTS.md, RULES.md, and DECISIONS.md.

I want to move to the next planned backend milestone: campaign approval, PostgreSQL-backed delivery queue, and mock delivery worker.

First, verify what has actually been implemented and identify the exact next slice in the original implementation plan. Do not invent additional slices or expand the scope unnecessarily.

Plan the next increment with these business requirements:

1. A user can approve a campaign only when its segment and saved message templates are valid.
2. Approval must use current consent-aware audience data and protect against stale campaign revisions.
3. Approved campaigns can be enqueued for simulated SMS/email delivery.
4. PostgreSQL is the source of truth for queue state.
5. A separate worker processes delivery jobs using a mock provider. No real SMS or email delivery.
6. Jobs must support safe retries, idempotency, and recovery from worker crashes.
7. Delivery progress and results must be queryable for the future dashboard.

Preserve existing campaign validation, revision handling, HTTP conventions, and mock-first testing.

Break the implementation into small, independently testable increments. Explain the database transactions, state transitions, API endpoints, failure handling, and tests.

Keep the architecture simple and aligned with the case study. Avoid unnecessary abstractions and premature optimization.


## 19. Assistant (2026-10-22T08:54:51.442Z)

I’ll compare the current code with the case study, implementation plan, and ADRs, then plan the next backend milestone. I won’t modify files or start implementation.


## 20. Assistant (2026-10-22T08:55:20.641Z)

The plan lists campaign approval next, followed by the PostgreSQL delivery queue, worker, retries, idempotency, and fault-injecting mock provider. I’ll break those existing milestones into testable implementation steps without adding new slice names.

The delivery skill and ADR-014 call for approval and enqueueing in one transaction. I’ll use that as the baseline and address a gap in the current saved-message preview: aggregate counts cannot detect a changed audience when different recipients produce the same counts.


## 21. Assistant (2026-10-22T08:58:21.413Z)

`make check` passed on the current tree. The code has saved-message validation, revision preconditions, and preview-only generation, but no approval route, deliveries table, worker, or mock-provider service. The `approved` state appears in schema constraints and test fixtures, not in an implemented approval flow.

I’m applying the delivery-pipeline skill alongside the campaign-domain and simplicity guidance. The plan will keep PostgreSQL as the queue, atomically approve and enqueue, and freeze the recipient payloads so retries cannot change approved content.


## 22. Assistant (2026-10-22T09:00:43.448Z)

**The next planned work is campaign approval, followed by the PostgreSQL delivery queue and mock worker.** These are already listed under “Later slices” in [IMPLEMENTATION_PLAN.md](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/IMPLEMENTATION_PLAN.md). The document does not assign them a new slice name, so I would keep those milestone names and divide their implementation into reviewable increments.

No files were changed.

**What actually exists**

| Implemented | Evidence |
|---|---|
| Validated, consent-filtered segment queries | `segment.Validated.Query` and `previewInTx` |
| Saved templates and human editing | `Campaigns.PutMessages` / `GetMessages` |
| Save-time validation against fresh audience bounds | `PutMessages` uses a repeatable-read transaction and `ValidateMessages` |
| Campaign-wide revision preconditions | `requiredRevision`, conditional segment/message writes |
| Retained text with invalidated readiness after segment changes | `PutSegment` clears validation metadata and returns `draft` |
| Preview-only message generation | Final SSE includes `base_revision`; no persistence |

Relevant implementations: [campaign storage](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/store/campaign.go), [message storage](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/store/messages.go), and [audience queries](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/internal/store/audience.go).

**Not implemented:** approval, enqueueing, deliveries, claiming jobs, delivery retries, a worker executable, a mock-provider executable, or delivery progress endpoints. The schema already permits later campaign states and contains `approved_at`, but that is not an implemented approval flow. Compose currently runs API/PostgreSQL, plus the optional test service.

I reran `make check`; it passed:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

The prior Docker/Postman evidence remains recorded in README; those suites were not rerun for this planning turn.

**Architecture to retain**

Use the existing ADRs:

- ADR-006: `deliveries` is the PostgreSQL queue.
- ADR-007: unique campaign/recipient/channel rows plus provider idempotency.
- ADR-009: bounded retries.
- ADR-013: paced per-channel dispatch and a final consent check.
- ADR-014: approval-time validation and frozen delivery payloads.
- ADR-022: revision protection and immutable content after approval.

These delivery decisions remain **Proposed** until implemented and verified. See [DECISIONS.md](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/DECISIONS.md).

I recommend **one public approval operation that atomically enqueues**. This follows the delivery skill and ADR-014, avoids an approved-but-not-enqueued failure window, and needs no separate enqueue endpoint.

### 1. Approval, frozen payloads, and queue insertion

This is the recommended first implementation increment. It can be tested without running a worker.

**Handle audience changes explicitly**

The current preview stores counts and maximum lengths. Those cannot detect two different audiences with identical counts, or changed personalized text of the same length.

Extend the existing `POST /campaigns/{id}/audience-preview` for `content_ready` campaigns to return:

- Current campaign revision.
- Existing aggregate preview.
- An opaque `approval_fingerprint` covering the sorted recipient/channel set, frozen destinations, rendered messages, and campaign revision.

Exclude the preview timestamp from the fingerprint: time passing alone must not invalidate approval when the resulting audience and payloads remain identical. This token is a comparison value, not authentication.

No new preview endpoint, historical snapshots table, or signing infrastructure is necessary.

**Approval contract**

```http
POST /campaigns/{id}/approve
Content-Type: application/json
If-Match: "campaign-42-r7"

{"approval_fingerprint":"<value returned by audience-preview>"}
```

Approval transaction:

1. Begin a deadline-bound repeatable-read transaction.
2. Lock the campaign and verify `If-Match`.
3. Require `content_ready`, a valid segment, and complete saved templates validated for that revision.
4. Recompute the current consent-aware audience using one UTC `as_of`.
5. Reuse `ValidateMessages` with both independent rendering bounds.
6. Reject an empty overall audience.
7. Render and freeze each recipient/channel destination, subject, and body.
8. Compare the fingerprint with what the user reviewed.
9. Insert delivery rows, set `approved_at`, advance campaign revision, and set `sending`.
10. Commit everything together.

Use bounded bulk insertion rather than a database round trip per recipient. No HTTP or LLM call occurs inside this transaction.

If the fingerprint differs, return **409 `approval_preview_changed`**, with updated aggregate preview/fingerprint, and insert nothing. The caller must review and explicitly resubmit.

**States and repeat requests**

The conceptual transition is:

```text
content_ready → approved → sending
```

Approval and queue insertion occur in one transaction; the externally visible committed state is `sending`. `approved_at` records approval. There is no separate waiting/enqueue state.

Preserve strict revision handling:

- Missing token: [REDACTED]
- Malformed token: [REDACTED]
- Stale token: [REDACTED]
- Invalid state / empty audience / changed preview: **409**
- Invalid segment/templates: existing validation errors
- Database failure: **503**, with full rollback

A repeated request cannot create additional jobs. An old token still returns 412; after reconciliation, an already-approved campaign can return its existing approval result without enqueueing again.

### 2. Database changes

Add an additive migration after 006; leave existing migrations untouched.

`deliveries` should contain:

| Purpose | Fields |
|---|---|
| Identity | `id`, `campaign_id`, `user_id`, `channel` |
| Frozen approval payload | destination, subject, body |
| Queue state | `status`, `attempts`, `next_attempt_at` |
| Claim ownership | `locked_at`, `lease_expires_at`, claim generation |
| Result | `provider_message_id`, safe `last_error_code` |
| Audit metadata | deterministic `idempotency_key`, UTC timestamps |

Required constraints:

- `UNIQUE (campaign_id, user_id, channel)`
- Unique idempotency key.
- Channel/status allowlists.
- Appropriate foreign keys and nonnegative attempt count.
- Channel-specific payload shape.
- An index supporting due pending jobs by channel/time.

Persist the approved source revision/fingerprint on the campaign so approval results remain identifiable.

Frozen destinations and personalized messages stay in PostgreSQL and mock HTTP requests—not logs, progress responses, LLM prompts, or session records. Workers never rebuild payloads from mutable profiles.

**First-increment tests**

- Draft, missing segment, missing channel, invalid template, and empty-audience rejection.
- Fresh audience/profile/consent changes reflected at approval.
- Same-count audience changes detected.
- Stale revision and concurrent edit-versus-approve rejection.
- Concurrent/repeated approval creates one delivery per recipient/channel.
- Failed insertion rolls back approval and every delivery row.
- Frozen payloads survive subsequent profile changes.

### 3. Separate mock provider and happy-path worker

Build `cmd/mockprovider` and `cmd/worker`, using standard HTTP and the existing PostgreSQL driver.

The mock provider should:

- Accept simulated SMS/email requests only.
- Honour the deterministic key:
  `campaign:{campaign_id}:user:{user_id}:{channel}`.
- Return the original receipt for a repeated identical request.
- Reject reuse of a key with different content.
- Persist its acceptance ledger in PostgreSQL so provider restarts preserve deduplication.
- Expose aggregate debug counts without recipient data.

The ledger is evidence of mock acceptance; **delivery queue state remains authoritative for application progress**.

Worker flow:

1. Claim one due job using `FOR UPDATE SKIP LOCKED`.
2. Commit the claim before making an HTTP request.
3. Wait for per-channel limiter capacity.
4. Check claim ownership and current channel consent.
5. Call only the allowlisted mock-provider URL.
6. Persist the outcome using a conditional update tied to the claim generation.

Claiming one job at a time keeps lease management simple and avoids jobs expiring while waiting in a claimed batch.

Use one worker process with one serial loop per channel. Preserve ADR-013’s single-owner-per-channel deployment constraint; multiple replicas are outside this increment.

**Rate and consent requirements belong in this first sending implementation**, not later hardening:

- No accumulated burst credit.
- At least 10 ms between starts per channel, including retries.
- No more than 100 starts in a half-open rolling second.
- Consent checked after the limiter wait.
- Missing/revoked consent → `skipped`, with no provider call.
- Consent lookup failure → no provider call.

The small race after the final consent check remains the limitation already documented in ADR-013.

### 4. Retries and crash recovery

Extend the working path with the already-planned retry policy:

| Outcome | Action |
|---|---|
| Successful acceptance | `sent` |
| Missing/revoked consent | `skipped` |
| Network error, timeout, 5xx, 429 | Retry within budget |
| Other 4xx | `dead` |
| Retry budget exhausted | `dead`, with safe reason |

Use the existing planned maximum of five attempts and full-jitter bounded exponential backoff. Honour valid `Retry-After`; neither worker nor HTTP client should add a second hidden retry mechanism.

A reaper returns expired claims to `pending`, retaining the idempotency key. All acknowledgement updates must match the claim generation, so a delayed former owner cannot overwrite a newer result.

For ambiguous final outcomes—such as provider acceptance followed by a lost response—use the durable mock receipt lookup to reconcile before declaring success or final failure. If acceptance cannot be confirmed, expose an explicit unknown-outcome error rather than claiming `sent`.

No database transaction spans provider HTTP.

Delivery states:

```text
pending → in_flight → sent
                   → skipped
                   → dead
                   → pending (retry or expired lease)
```

A campaign becomes `completed` when no pending/in-flight rows remain. Completion can include failures or skips; it does not mean every message was sent. Serialize the terminal-result/completion check on the campaign row so concurrent workers cannot leave the final campaign stuck in `sending`.

**Tests**

- Crash after claim, before request.
- Acceptance followed by lost response.
- Crash after acceptance, before database acknowledgement.
- Expired lease and stale acknowledgement.
- Provider restart preserves idempotency.
- Attempt-five exhaustion and ambiguous-outcome reconciliation.
- 429 timing, permanent 4xx, network and 5xx recovery.
- Fake-clock limiter tests, including delayed consent checks.
- Competing claimers never create duplicate accepted effects.

### 5. Queryable progress and Compose integration

Add:

| Endpoint | Response |
|---|---|
| `GET /campaigns/{id}/delivery-status` | Campaign state and counts by channel/status |
| `GET /campaigns/{id}/deliveries?after_id=…&limit=…` | Bounded, cursor-paginated result metadata |

Return delivery ID, channel, status, attempts, timestamps, provider receipt ID, and safe error code. Exclude destinations and rendered payloads.

Compute progress from PostgreSQL in a consistent read snapshot. Do not maintain authoritative in-memory counters. Keep delivery progress separate from the campaign editing revision.

The aggregate status endpoint can ship with the first approval/queue increment; pagination follows once worker results exist.

Add worker and mock provider to ordinary Compose startup. Preserve the API’s mock LLM default. Fault configuration should follow the existing delivery skill’s seeded 5xx/429/latency controls.

Dashboard UI, progress SSE, and metrics remain in their existing later milestone; polling these read endpoints is sufficient here.

**Implementation order and files**

These are implementation increments within the existing milestones—not additional slices:

| Increment | Main files |
|---|---|
| Approval/queue, no worker | New migration; campaign rendering helpers; store approval transaction; HTTP approval/status routes; existing audience-preview contract; tests/Postman |
| Happy-path worker/provider | `internal/delivery`, store claim/result methods, `cmd/worker`, `cmd/mockprovider`, provider receipt migration, config/Docker/Compose |
| Retry/crash recovery | Retry scheduling, leases/reaper, fenced updates, receipt reconciliation, deterministic fault tests |
| Results and milestone verification | Paginated result route, completion checks, Compose end-to-end and Postman coverage |

Each increment should update README, IMPLEMENTATION_PLAN, relevant skills/ADRs, and affected Postman requests alongside the code. Reuse existing validators, parameterized query construction, revision parsing, safe errors, and transaction helpers.

**Recommended next step:** approve only the **approval/queue increment** first: current-audience preview fingerprint, transactional approval plus frozen delivery rows, and aggregate status retrieval—with no running worker yet. This gives us an independently testable boundary before simulated sending begins.

Implementation requires the plan approval specified by [AGENTS.md](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/AGENTS.md): “Plans that touch more than two files or segment, LLM or delivery logic require approval before implementation.”

## 23. User (2026-10-22T09:03:41.726Z)

do the recommended action. approval/queue 


## 24. Assistant (2026-10-22T09:03:48.730Z)

I’ll implement the approved approval/queue increment: a current-audience approval fingerprint, atomic approval and queue insertion with frozen payloads, and aggregate delivery status. I’ll leave the worker and mock provider for the next increment and won’t commit changes.


## 25. Assistant (2026-10-22T09:04:09.102Z)

I’m applying the campaign-domain, delivery-pipeline, verification, Ponytail, and Postman testing guidance. I’ll reuse the existing audience-preview endpoint for the approval fingerprint; no separate enqueue endpoint is needed.


## 26. Assistant (2026-10-22T09:08:25.958Z)

The storage path is in place: approval recomputes consent-aware recipients, revalidates and renders saved templates, compares the reviewed fingerprint, then inserts pending jobs and marks the campaign `sending` in one transaction. I’m adding the HTTP contracts and tests now, including audience changes that leave counts unchanged.


## 27. Assistant (2026-10-22T09:15:36.650Z)

`make check` and the PostgreSQL suite passed. The database tests cover same-count audience changes, frozen payloads, repeated approval, competing writes, rollback across insertion batches, cancellation, and approval over the 5,000-user seed. I’ll now run the HTTP lifecycle through Postman against the local mock/test API.


## 28. User (2026-10-22T11:01:18.874Z)

continue


## 29. Assistant (2026-10-22T11:01:23.461Z)

Continuing the approval/queue increment. The database checks are green; I’m running Postman, then I’ll finish the ADRs, documentation, and diff review.


## 30. Assistant (2026-10-22T11:03:41.527Z)

Postman passed all 97 requests and 232 assertions. Approval and queue insertion commit together, repeated approval creates no duplicates, and jobs remain pending without a worker. I’m finishing the documentation and reviewing the diff.


## 31. Assistant (2026-10-22T11:06:30.860Z)

The rendering regression failed when I deliberately enabled recursive substitution, then passed after I restored the source. Final checks are running. The docs now clarify that `sending` means queued in this increment; worker retries and crash recovery still need implementation and verification.


## 32. Assistant (2026-10-22T11:06:53.647Z)

Implemented the approved **campaign approval and PostgreSQL queue increment**. No commit created.

The runtime flow is now:

**Generate preview → explicitly save/edit → revalidate → content_ready → review audience → approve and enqueue atomically → sending**

Approval locks the campaign, checks its revision, recomputes current consent-aware audience bounds, revalidates templates, and freezes personalized delivery payloads. Queue insertion and the campaign transition commit together.

New contracts:

- `POST /campaigns/{id}/audience-preview` now includes a revision and approval fingerprint.
- `POST /campaigns/{id}/approve` requires that fingerprint and `If-Match`.
- `GET /campaigns/{id}/delivery-status` returns PostgreSQL aggregate counts without recipient details.

Stale revisions return **412**. Changed audience/content requires renewed review with **409**. Reconciled repeated approval creates no duplicate jobs.

Verification passed:

- `make check`: safety/formatting, vet and unit tests.
- Docker PostgreSQL integration tests, including concurrency, rollback, cancellation and 5000-user coverage.
- Local Postman: **97 requests, 232 assertions, zero failures**.
- A deliberate rendering defect failed its regression test; restored code passed.

For Postman, run the collection in order, then follow **Campaign approval and queue**: save messages, fetch the audience preview, approve using its ETag/fingerprint, and query delivery status. Details are in [the Postman walkthrough](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/postman/README.md).

Jobs remain **pending**: worker execution, retries, crash recovery and the mock delivery provider are the next increment. No real Anthropic, SMS or email calls were made.

## 33. User (2026-10-22T11:09:15.078Z)

Proceed with the next planned backend increment: **PostgreSQL Queue Worker and Mock Delivery**.

The previous increment implemented atomic campaign approval and queue insertion. Delivery jobs are currently pending.

Review the existing implementation and project documentation before making changes:

- AGENTS.md, CLAUDE.md, RULES.md, DECISIONS.md
- IMPLEMENTATION_PLAN.md
- Existing queue schema, approval logic, and delivery-status API

Implement the worker according to the original case-study requirements.

### Requirements

1. Implement a separate Go worker process (`cmd/worker`) that consumes PostgreSQL delivery jobs.
2. Support safe concurrent workers using PostgreSQL row locking (`FOR UPDATE SKIP LOCKED`) or an equivalent reliable approach.
3. Use the existing frozen delivery payloads. Never regenerate messages or re-query recipients during delivery.
4. Implement a deterministic mock delivery provider. No real SMS or email requests.
5. Track job lifecycle: pending, processing, delivered, failed, with bounded retries.
6. Implement retryable versus permanent failures, retry limits, and backoff.
7. Recover jobs abandoned by crashed workers using leases or an equivalent mechanism.
8. Prevent concurrent workers from processing the same active job. Use idempotency and fencing where appropriate.
9. Update campaign delivery status consistently as jobs complete.
10. Support graceful shutdown, context cancellation, and configurable worker concurrency.

### Verification

Add tests covering:

- Successful delivery
- Transient failure followed by successful retry
- Permanent failure
- Retry exhaustion
- Concurrent workers
- Worker crash and lease recovery
- Duplicate processing and idempotency
- Cancellation and graceful shutdown
- PostgreSQL transaction failures
- Existing approval and queue regressions

Run the existing Go checks, PostgreSQL integration tests, Docker Compose tests, and Postman collection.

### Constraints

- Preserve existing API contracts and campaign revision semantics.
- Reuse existing database and domain patterns.
- Do not introduce Redis, Kafka, RabbitMQ, or external queue infrastructure.
- Do not send real SMS/email or make real Anthropic API calls.
- Do not implement the Vue frontend yet.
- Avoid unnecessary abstractions and scope expansion.
- Do not commit without my approval.

Implement this increment, run the available tests, and report the results.

At the end, explain:

1. How jobs move through the queue.
2. How concurrent workers avoid conflicts.
3. How retries and crash recovery work.
4. How to run and verify the worker locally.
5. What remains before starting the Vue frontend.


## 34. Assistant (2026-10-22T11:09:19.568Z)

I’ll review the queue implementation and project rules, then prepare the worker plan. The repository requires approval of a concrete delivery-logic plan before edits; I’ll check whether the requested lifecycle terms fit the existing API contract as part of that review.


## 35. Assistant (2026-10-22T11:09:29.301Z)

The queue already has lease and fencing fields, frozen payloads, and these status names: `pending`, `in_flight`, `sent`, `dead`, `skipped`. I’ll preserve those API names—processing, delivered and failed map to `in_flight`, `sent` and `dead`. The original requirements also require a 100-per-second channel limit and a final consent check before sending; neither requires rebuilding the audience.


## 36. Assistant (2026-10-22T11:09:45.943Z)

I’m applying the delivery-pipeline, campaign-domain, verification, Ponytail and official Postman testing skills. One gap needs to be addressed in the worker design: the existing limiter decision assumes one process per channel. With concurrent workers, I propose a shared PostgreSQL channel gate so extra processes cannot multiply the 100-per-second limit.


## 37. Assistant (2026-10-22T11:10:17.162Z)

The next planned milestone is the existing **PostgreSQL delivery worker, retries, idempotency and fault-injecting mock provider** milestone. Approval, frozen payloads, queue uniqueness, leases and fencing columns already exist. No files changed during this review.

I propose these independently testable implementation steps:

1. **Queue operations**
   - Add `internal/store/delivery.go` and PostgreSQL tests.
   - Claim due jobs with `FOR UPDATE SKIP LOCKED`, set a lease and increment `claim_generation`.
   - Fence every result update using the job ID and claim generation. Reject stale acknowledgements.
   - Recover expired leases without exceeding the retry limit.

2. **Mock provider**
   - Add `cmd/mockprovider`, a small HTTP implementation and tests.
   - Support deterministic faults, delays, 429 responses and permanent failures.
   - Persist accepted idempotency keys and payload hashes in PostgreSQL so provider restarts cannot cause duplicate simulated acceptance.
   - Return aggregate debug counts without exposing recipients or content.

3. **Worker**
   - Add `cmd/worker` and `internal/delivery`.
   - Send only frozen destinations and payloads.
   - Recheck consent, campaign state and claim ownership immediately before dispatch; never rebuild the audience.
   - Retry network errors, timeouts, 5xx and 429, respecting `Retry-After`; other 4xx fail permanently.
   - Use the existing five-attempt limit and capped exponential backoff with jitter.
   - Support configurable concurrency, signal-driven shutdown and bounded cancellation.

4. **Concurrency and completion**
   - Add a PostgreSQL-backed channel admission gate to preserve the case-study limit of **100 calls per second per channel across worker processes**. This updates the earlier single-process limiter assumption.
   - Keep transactions short; none span provider HTTP calls.
   - Atomically record outcomes and mark campaigns `completed` when no pending or active jobs remain. Delivery progress will not increment campaign revisions.

5. **Runtime and contracts**
   - Update Dockerfile, Compose, configuration and `.env.example`.
   - Preserve existing status names: `pending → in_flight → sent/dead`, with retries returning to `pending` and revoked consent producing `skipped`.
   - Preserve the delivery-status response shape.
   - Update Postman assertions to accommodate active processing and add a completion workflow.

6. **Verification and documentation**
   - Test success, retries, exhaustion, permanent failures, concurrent claims, expired leases, stale fencing, duplicate acceptance, cancellation, shutdown and database failures.
   - Run Go checks, PostgreSQL integration tests, Compose verification and local Postman regressions.
   - Update ADRs, implementation progress and local run instructions. No frontend, real delivery, real Anthropic calls or commits.

**Please approve this file and transaction plan before implementation.** [AGENTS.md](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/AGENTS.md) explicitly requires: “For changes touching more than two files, or … delivery logic, propose a plan and wait for approval.” The shared channel gate and durable mock acceptance ledger are the concrete decisions requiring approval.

## 38. User (2026-10-22T11:10:34.326Z)

approved


## 39. Assistant (2026-10-22T11:10:37.129Z)

I’ll start with the queue transactions and mock provider, verify them, then connect the worker and Compose services. I’ll preserve the existing API status names and campaign revision semantics.


## 40. Assistant (2026-10-22T11:13:17.682Z)

Queue claims, fenced result updates and lease recovery are in place. The mock provider now persists acceptance receipts without storing recipient payloads, allowing it to deduplicate a retry after either process restarts. Next I’ll wire up both processes and test failure and recovery paths.


## 41. Assistant (2026-10-22T11:18:33.830Z)

The local HTTP tests passed after allowing loopback access; the first run was blocked by the sandbox. PostgreSQL integration tests are now building. During review, I also aligned recovery and acknowledgement transactions to lock the campaign before delivery rows, avoiding a lock-order deadlock.


## 42. Assistant (2026-10-22T11:22:56.536Z)

The PostgreSQL suite passed, including concurrent workers and recovery after provider acceptance but before acknowledgement. A deliberate client mutation also made the permanent-failure tests fail; the restored client passed.

I’m checking one remaining edge case before the Compose run: a worker waiting for channel admission must renew its claim so it doesn’t exhaust retries without sending.


## 43. Assistant (2026-10-22T11:26:50.058Z)

Postman passed against the running API, worker and mock provider: **217 requests and 593 assertions, zero failures**. It observed campaign completion with no revision change. The PostgreSQL suite also passed the shared admission and stale-owner checks. I’m finishing the fail-closed tests, restart/shutdown checks and diff review.


## 44. Assistant (2026-10-22T11:31:27.621Z)

The Docker hard-stop test passed: `SIGKILL` left an active lease, and after restarting the worker and mock provider, the campaign completed with **1,597 simulated sends** and the same revision. Aggregate provider acceptances matched sent rows. Final tests are now exercising lease renewal during admission waits and the rolling channel limit.


## 45. Assistant (2026-10-22T11:35:26.523Z)

Final Postman verification passed: **203 requests and 548 assertions, zero failures**, including the mock provider contracts. Both worker containers shut down with exit code **0**, and I restored the stack to one worker. Documentation and ADR updates are complete; only the final repository checks remain.


## 46. Assistant (2026-10-22T11:36:05.999Z)

Implemented **PostgreSQL Queue Worker and Mock Delivery**. No commit created, and no real Anthropic, SMS or email calls were made.

- **Queue flow:** `pending → in_flight → sent`, with `dead` for permanent/exhausted failures and `skipped` for revoked consent. Delivery uses frozen payloads. Campaigns become `completed` when all jobs are terminal, without changing campaign revision.
- **Concurrency:** `FOR UPDATE SKIP LOCKED` prevents competing claims. Generation fencing rejects stale dispatches and acknowledgements. Shared PostgreSQL channel admission preserves the 100-per-second limit across worker processes.
- **Retries and recovery:** Network failures, timeouts, 429 and 5xx retry with jittered backoff and `Retry-After`. Processing is capped at five attempts. Expired leases recover abandoned jobs; waiting workers renew leases. The mock’s durable acceptance ledger prevents duplicate simulated acceptance across restarts.
- **Shutdown:** Signals cancel waits and HTTP calls, stop new claims and leave ambiguous work recoverable.

Verification passed:

```text
repository safety and formatting checks passed
go vet -mod=readonly ./...
go test -mod=readonly ./...
```

PostgreSQL integration tests passed, including concurrency, retries, exhaustion, fencing, transaction failures, consent checks and lease renewal. Final Postman execution passed **203 requests and 548 assertions**, with zero failures. A Docker `SIGKILL` test recovered an abandoned lease and completed **1,597 jobs** with unchanged revision; aggregate acceptances matched sent rows. Both worker containers shut down cleanly with exit code **0**.

Run locally:

```powershell
docker compose --env-file .env.example up -d --build
```

Approve a synthetic campaign, then query `GET /campaigns/{id}/delivery-status`. Run the maintained Postman collection with the worker active; its final workflow polls through completion. Detailed commands and configuration are in [README.md](C:/Users/USER/Desktop/InsiderOne_[REDACTED_NAME][REDACTED_NAME]_CaseStudy/SmartCampaignComposer/README.md).

One documented limit remains: a provider-accepted final attempt can become `dead` if the worker crashes before acknowledgement. Deduplication prevents another acceptance, but does not guarantee exact outcome reconciliation.

The next planned milestone is **Vue, dashboard and metrics**. Existing APIs support starting the frontend; metrics endpoints and dashboard push updates remain unfinished. The local stack is restored to one worker with fault injection disabled for verification.
