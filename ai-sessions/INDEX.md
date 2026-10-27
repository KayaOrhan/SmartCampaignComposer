# AI development session evidence

Development used Codex. On 2026-10-23 the author confirmed no separate Claude Code or claude.ai sessions were used. These files are authentic Codex conversation exports, not fabricated Claude records. The case study permits AI tools such as Cursor, while its export wording specifically names Claude; that tool difference is disclosed for the reviewer.

## Export inventory

| Export | Original chat and coverage |
| --- | --- |
| [2026-10-21-foundation-domain-llm.md](2026-10-21-foundation-domain-llm.md) | Contract alignment, foundation/domain/seed, persistence repair, mock generation, SSE and official SDK; work spans 2026-10-21/22 |
| [2026-10-21-foundation-review.md](2026-10-21-foundation-review.md) | Critical read-only review, including ignored evidence, scanner/test/setup limitations |
| [2026-10-21-domain-review.md](2026-10-21-domain-review.md) | Persistence-limit defect and database snapshot test gap |
| [2026-10-21-postman.md](2026-10-21-postman.md) | Collection plan/approval, execution and failing assertion control |
| [2026-10-22-persistence-approval-worker.md](2026-10-22-persistence-approval-worker.md) | SDK review/model correction, explicit saves/revisions, approval/queue and concurrent worker/mock delivery |
| [2026-10-22-vue-dashboard-metrics.md](2026-10-22-vue-dashboard-metrics.md) | Frontend plan/approval, implementation, browser/Postman, screenshot corrections and Docker recovery |
| [2026-10-23-submission-review.md](2026-10-23-submission-review.md) | Current review snapshot before final response; final evidence is in docs/SUBMISSION_REVIEW.md |

Each file records its source conversation ID and original source SHA-256 (the hash identifies the source bytes, not an externally verifiable archive). Export dates follow source-chat creation; original UTC message timestamps are retained. Public user prompts and assistant commentary/final messages are chronological. Injected instructions, internal reasoning, tool traffic and compaction summaries are omitted explicitly; these are public conversation transcripts, not complete machine-event logs. No dialogue or results were invented. Historical reports may describe earlier defects later fixed; consult the final review for current findings.

Raw export inputs and temporary review tooling are under ignored raw/. The required .claude/hooks/scrub.sh ran with Git Bash/Perl, followed by manual redaction of identifying path/account fragments and review of public text. The original source files remain outside the submission. Only sanitized Markdown is eligible for a future explicitly reviewed commit.

## Final review discipline (2026-10-23)

- Delegated: No subagents; used local checks and the official Postman testing workflow.
- Kept: Approved documentation/export scope, existing implementation changes, synthetic mock-only calls and no commit.
- Where the agent was wrong: An oversized combined read truncated documentation output; files were read again in bounded sections. Initial sandbox browser/uncached Go and Git Bash attempts could not access their local runtime resources.
- How I caught it: Tool truncation/errors, bounded re-reads, approved local reruns, all seven transcript provenance records, fresh-database workflow, full PostgreSQL/browser/Postman checks and final diff inspection. Current published HEAD still lacks the uncommitted frontend; no final remote-clone success is claimed.

## Historical slice notes

The notes below were written during development. Their former export-pending labels are resolved by the inventory above; past tooling limitations remain historical observations, not current test results.

## 2026-10-22 Vue frontend, dashboard and metrics (source transcript in inventory)
- Delegated: No subagents; used approved incremental Vue/API work and the official Postman workflow locally.
- Kept: Explicit saves, original candidate revisions, reviewed approval fingerprints, mock-only runtime, aggregate privacy and no commits.
- Where the agent was wrong: Initially used an unsupported Postman folder flag; corrected to the documented item selector. Screenshot review found a stale delivery notice and a mobile creation form below the long list.
- How I caught it: CLI failure, desktop/mobile screenshots, frontend/Go/PostgreSQL checks, browser lifecycle and stale-save tests, full local Postman, and a deliberate missing-review-guard mutation that failed before restoration. Source transcript is now in the export inventory; raw-only ignore policy is documented above.

One entry per exported session. Written at the end of the session, while it is fresh.

<!--
## YYYY-MM-DD <slice name> (file: YYYY-MM-DD-<slice>.md)
- Delegated:
- Kept:
- Where the agent was wrong:
- How I caught it:
-->

## 2026-10-21 Contract alignment (source transcript in inventory)
- Delegated: No subagents; agent edited documentation and drafted the runtime plan.
- Kept: User authorized contract changes; runtime plan awaits approval.
- Where the agent was wrong: Initial inventory omitted ignored template files and empty directories; the scaffold audit corrected this before its final report.
- How I caught it: Explicit directory listing and safe inspection of .env.example; no application execution.

## 2026-10-21 Slice 1 backend foundation (source transcript in inventory)
- Delegated: No subagents; implemented approved Go API, PostgreSQL pool, small migrations, Compose, checks and tests.
- Kept: User approved scope; no campaign, LLM, worker or frontend work and no commits.
- Where the agent was wrong: A deliberate liveness mutation overlapped a full test run, contaminating that run; repeated the suite after restoring source.
- How I caught it: Endpoint test failed as intended; restored-source tests and PostgreSQL integration passed, then HTTP outage/recovery probes confirmed 200/503 behavior. Make and race checks remain blocked by missing local tools.

## 2026-10-21 Slice 2 domain, seed and segments (source transcript in inventory)
- Delegated: No subagents; implemented the approved schema, draft API, deterministic seed and validated segment engine.
- Kept: User approved the scope; preserved existing AGENTS.md edits and PostgreSQL queue architecture; no LLM, delivery work or commits.
- Where the agent was wrong: Initial JSON decoding would have accepted case-insensitive aliases; added exact-key checks before verification. The bundled skill validator initially lacked PyYAML; installed a verified pinned copy into temporary tooling storage.
- How I caught it: Case-mismatch tests, successful make check, PostgreSQL boundary/consent/seed tests, deliberately broken consent-filter detection, live API/restart checks and the bundled skill validator. No real secrets or recipient contacts exported.

## 2026-10-21 Postman API testing (source transcript in inventory)
- Delegated: No subagents; used official Postman plugin skills and CLI for the approved local collection.
- Kept: Preserved existing changes and the existing AGENTS.md API policy; no custom skill, API behavior changes or commits.
- Where the agent was wrong: The initial CLI run lacked sandbox permission to reach localhost; collection lint scanned zero v2.1 JSON files and cannot establish schema validation.
- How I caught it: Approved local CLI run passed 21 requests and 62 assertions with exit 0; deliberate wrong-status control exited 1, temporary copy removed, and make check passed.

## 2026-10-22 Slice 2.1 persistence repair (source transcript in inventory)
- Delegated: No subagents; approved segment persistence repair only.
- Kept: HTTP ingress limit, complete DSL validation, PostgreSQL queue decision and no commits.
- Where the agent was wrong: The original implementation reapplied a transport limit to expanded stored documents.
- How I caught it: Compact HTML escaping reproduction; added canonical/JSONB round-trip and HTTP/Postman regression checks. Execution evidence is recorded in README.

## 2026-10-22 Slice 3A mock brief-to-segment (source transcript in inventory)
- Delegated: No subagents; implemented the approved mock generation, audit and API slice.
- Kept: Existing DSL validation, draft state checks, key-free tests and PostgreSQL architecture; no real Claude, messages, streaming, delivery or commits.
- Where the agent was wrong: Review found that a fixed HTTP write timeout could expire before the configured generation budget; derived it from generation and database deadlines.
- How I caught it: Deadline/config tests, deliberately broken output-validation control, PostgreSQL audit/persistence tests and local Postman execution; evidence in README. The existing gitignore still excludes session records.

## 2026-10-22 Slice 3B mock message generation and SSE (source transcript in inventory)
- Delegated: No subagents; implemented the approved message validation, mock streaming, shared call runner and SSE endpoint.
- Kept: Combined worst-case bounds, key-free fixtures, sanitized audit metadata and preview-only state; no real Claude, persistence, approval, delivery or commits.
- Where the agent was wrong: Initial real-socket tests ran inside a sandbox that blocks loopback; reran with approved local-network access without skipping tests.
- How I caught it: Both deliberate worst-case/Flush mutations failed their regressions; full Go, PostgreSQL and Postman checks passed. Review also rejected empty display chunks and cancels immediately after failed writes. Evidence is in README; session records remain ignored by the existing gitignore.

## 2026-10-22 Slice 3C official Claude adapter (source transcript in inventory)
- Delegated: No subagents; implemented the approved SDK adapter, bounded tool-stream decoder and runtime configuration.
- Kept: Mock defaults, independent test guards, strict validators, bounded retries/repairs and sanitized audit metadata; no real key/API call, persistence/approval, delivery or commits.
- Where the agent was wrong: The first SDK cancellation fixture failed to consume the HTTP request body; corrected it and asserted cancellation followed a display delta. A missing-stop fixture initially matched LF against CRLF; normalized only the test fixture mutation.
- How I caught it: Local SDK protocol/decoder tests, a deliberately removed terminal-stop guard that failed regression, full Go/module/PostgreSQL/CA-image checks and 49-request/131-assertion mock Postman execution. Live Claude remains unverified; evidence is in README and session records remain ignored by the existing gitignore.


## 2026-10-22 Slice 3D persistence and editing (source transcript in inventory)
- Delegated: No subagents; applied the approved incremental model/revision/message-persistence plan using the official Postman testing skill and CLI.
- Kept: Preview-only generation, fresh transactional bounds, campaign-wide revisions, preserved human text, mock defaults and no commits or real API calls.
- Where the agent was wrong: Initial sandbox HTTP checks could not connect to local sockets; reran with approved access. Review ensured SQL raw length ceilings allow placeholders longer than rendered content.
- How I caught it: Unit/HTTP and Docker PostgreSQL lifecycle/upgrade/rollback/concurrency tests, a deliberately removed precondition guard that failed regression, and 74-request/176-assertion local Postman verification. Evidence is in README; existing session-record ignore policy remains unchanged.
- Delegated (2026-10-22 approval/queue): No subagents; used the official Postman testing workflow locally.
- Kept: Approved scope, mock defaults, explicit save, revision preconditions, atomic consent-aware frozen queue, no worker or commits.
- Where the agent was wrong: No new implementation defect found in review; a deliberate recursive rendering mutation violated literal substitution.
- How I caught it: Rendering regression failed with the mutation; restored sources, unit/check, Docker PostgreSQL concurrency/rollback tests and local Postman (97 requests, 232 assertions) passed. Source transcript is in the export inventory.
- Delegated (2026-10-22 queue worker/mock delivery): No subagents; used local Go, Docker/PostgreSQL and official Postman testing workflows.
- Kept: Frozen payloads, consent/fencing boundaries, PostgreSQL queue/admission/deduplication, existing statuses/revisions, mock-only transport and no commits.
- Where the agent was wrong: Initial local HTTP checks were sandbox-blocked; review also caught lock-order, uncertain advisory-lock cleanup and waiting-lease expiry risks before completion.
- How I caught it: Approved local reruns, database failure/concurrency/rolling-window/renewal tests, deliberate classification failure control, SIGKILL recovery with aggregate deduplication check and final 203-request/548-assertion Postman run. Source transcript is in the export inventory.

## 2026-10-25 PostgreSQL 18.6 development upgrade
- Delegated: No subagents; used the Ponytail, verification and official Postman testing workflows.
- Kept: Existing application migrations, pgx, mock-only services and user edits; selected fresh PostgreSQL 18 storage without historical conversion or commits.
- Where the agent was wrong: The first verification attempt was interrupted when Docker Desktop stopped; no passing result was claimed for that run.
- How I caught it: Runtime version/path, migration and seed counts plus make check and rerun PostgreSQL integration verified the baseline; final API and persistence results are recorded in README.

## 2026-10-25 Focused campaign UX and safe draft deletion
- Delegated: No subagents; used campaign-domain, delivery-pipeline, Ponytail, verification and official Postman workflows for the approved plan.
- Kept: Explicit human confirmation, fresh revision/fingerprint review, restrictive foreign keys, retained audits/content, no delivery-history deletion, mock-only verification and no commits.
- Where the agent was wrong: The initial deletion file had an unused import, and the old collection still expected DELETE to be unsupported. Fixed the import and changed that assertion to PATCH; no failing check was bypassed.
- How I caught it: Go vet, PostgreSQL lifecycle/retention/concurrency tests, 21 frontend tests, six browser regressions and the final 234-request/596-assertion local Postman run. A temporary confirmation-guard mutation failed two tests; restored sources passed final checks. Canonical evidence and limitations are in README.

## 2026-10-25 Bilingual interface and OpenAPI documentation
- Delegated: No subagents; used campaign/delivery/LLM contracts, Ponytail, verification and official Postman documentation/testing workflows under the approved plan.
- Kept: Existing authored data, API machine contracts, explicit human review and deletion safeguards; English default, no i18n dependency, local-only Swagger assets, mock-only tests and no commits.
- Where the agent was wrong: The first automated text replacement retained v-pre around translated placeholder help; inspection caught and corrected it before browser verification. Review also guarded error-code lookup against object prototype names.
- How I caught it: 25 frontend tests, Go route/DTO/reference/Postman consistency, official OpenAPI/schema validation, PostgreSQL integration, nine browser regressions and 234-request/597-assertion local Postman execution. Translation and approval-documentation mutations failed the expected tests; restored final sources passed checks. Evidence and limitations are in README.

## 2026-10-25 Submission review documentation refresh
- Delegated: No subagents; documentation-only verification and comparison with current source, Git state, README evidence and export inventory.
- Kept: Existing committed implementation and safety rules; no runtime changes, application test reruns, publication or commits.
- Where the agent was wrong: The first combined read truncated the review; bounded reads recovered the full findings/checklist before editing. No new runtime defect was investigated.
- How I caught it: Actual Git status/HEAD tracked-file checks, current configuration and recorded verification exposed stale packaging, checkbox, test-count, database-exposure and export-completeness claims. Reviewed the documentation diff and validated local links; remaining submission TODOs are explicit in docs/SUBMISSION_REVIEW.md.

## 2026-10-27 Published-checkout submission verification
- Delegated: No subagents; used local verification and the official Postman testing workflow.
- Kept: User requested release-state verification only; preserved implementation, existing documentation refresh, original database volumes, mock-only services and explicit commit approval.
- Where the agent was wrong: The initial clean-checkout Compose invocation lacked sandbox access to Docker; no application defect was identified.
- How I caught it: Approved Docker access, a Git-clean remote checkout at c82409c, fresh-volume startup, 5000-user/nine-migration counts, make check, uncached Go tests, production/Docker builds, PostgreSQL integration, nine browser tests and 235-request/600-assertion local Postman passed. Restored the original stack; only the report/index require approved commit/publication. This is a verification note, not a conversation export.
