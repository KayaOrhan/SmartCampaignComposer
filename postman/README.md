# Local API regression collection

`SmartCampaignComposer.postman_collection.json` is a Postman Collection v2.1 artifact for the implemented Go HTTP API. Import it in Postman or run it with the official Postman CLI (verified with version 1.75.0). No workspace binding, login, API key, custom skill or additional runner dependency is required for this local collection.

From the repository root, start or rebuild the local API:

```powershell
$env:LLM_PROVIDER="mock"
$env:APP_ENV="test"
$env:ANTHROPIC_API_KEY=$null
docker compose --env-file .env.example up -d --build api
```

Wait until `http://127.0.0.1:8080/ready` returns HTTP 200. Startup applies migrations and seeds synthetic data once. Then run:

```powershell
postman collection run postman/SmartCampaignComposer.postman_collection.json --env-var "baseUrl=http://127.0.0.1:8080" --bail --timeout-request 5000 --no-report-events
```

The first requests assert liveness and database readiness, so an unavailable API fails the run. The CLI exits nonzero on failed assertions; do not add `--suppress-exit-code`. Run results are kept local with `--no-report-events`. The explicit Compose template avoids automatic loading of `.env`. Docker's Linux engine must be running.

Run the entire collection in order. `baseUrl` defaults to the loopback API; a collection pre-request guard permits only HTTP localhost/127.0.0.1 on port 8080. `campaignId` is captured from each successful create response in run-local scope, overriding stale collection/environment values for subsequent requests. Each full run creates one synthetic draft in the persistent development database. There is no delete endpoint, so the collection does not attempt cleanup or reset database volumes.

Coverage includes all implemented routes: health, readiness, create/get campaign, save segment, audience preview, segment generation and message streaming. Assertions cover canonical channels and segment values, draft persistence, Location, UTC timestamps, JSON/no-store headers, aggregate-only preview fields and union/count bounds. Negative cases check malformed/duplicate JSON, unknown fields, invalid channels/brief/ID, missing campaigns, content type, body size, forbidden consent filters, preview bodies and missing segments. A rejected segment update must preserve the stored segment. The missing-campaign check uses the maximum signed 64-bit ID as an absent fixture, never as a generated resource ID.

Audience counts are deliberately not fixed: seeded events age, and consent/profile edits are preserved. These HTTP assertions do not independently prove consent filtering; PostgreSQL domain tests cover that guarantee. Database-outage 503 and non-draft 409 cases remain covered by Go tests because this API cannot safely create those states through its public endpoints. The 3A folder covers mock segment generation, read/preview/repeat behavior and rejected body/ID/missing-campaign cases. Invalid model output, repairs, timeout, refusal and audit failures are covered by Go tests using internal scenario selection. The 3B folder parses finite SSE responses, reconstructs current-attempt fields, validates the final payload and rune limits, and checks repeat/unchanged-draft behavior and preflight errors. Message persistence/editing is covered by the 3D folder below; approval, delivery and metrics remain unimplemented. No real LLM or delivery service is called.

The canonical maintenance policy already exists in [AGENTS.md](../AGENTS.md#api-contract--postman-testing). Update this collection alongside API contract changes, using the official Postman plugin workflow. The Go handlers and domain contracts define implemented behavior.

Verification on 2026-10-21: the documented CLI command passed against the local Compose API at `http://127.0.0.1:8080`: 21 requests, 62 assertions, zero failures, exit 0. A temporary copy with a deliberately wrong health-status assertion failed with exit 1; it was removed without changing the maintained collection. `make check` passed (safety/format checks, vet and unit tests). The CLI emitted cloud-login notices but ran locally without login. `postman collection lint` reported `Scanned: 0` for this v2.1 JSON artifact, so it is not counted as schema-validation evidence; execution successfully parsed and ran all requests.

Slice 2.1 adds generated compact fixtures for HTML escaping and near-limit Unicode JSONB formatting. Each is saved, read, previewed and saved again. An oversized segment update must return 413 and preserve the prior document. Fixtures are generated in run-local variables, with no large committed payloads.

Verification on 2026-10-22: the documented local CLI command passed against the rebuilt Compose API: 31 requests, 82 assertions, zero failures, exit 0. No cloud reporting, real LLM or delivery calls were used.

3A verification on 2026-10-22: the documented local command passed 40 requests and 104 assertions, zero failures, exit 0. Generation scenarios that cannot be selected through the public API remain covered by Go unit/HTTP and PostgreSQL tests.

The SSE assertions require a terminal validated event; HTTP 200 alone is insufficient. The CLI buffers the finite stream before assertions, so this collection does not prove incremental flushing or disconnect behavior. Go HTTP tests use real local sockets and synchronization to prove those separately. Run the default happy scenario with a small chunk delay to stay within the documented five-second request timeout. Deliberately long startup scenarios belong in Go tests. Each new attempt resets reconstructed fields; templates are checked against both audience maxima, including empty-channel schema bounds.

3B verification on 2026-10-22: the documented local command passed 49 requests and 127 assertions, zero failures, exit 0. Go real-socket tests proved the incremental and cancellation behavior that buffered CLI assertions cannot establish.

Slice 3C keeps the public endpoints and SSE contract unchanged. The collection remains a deterministic mock-mode regression suite; it must not run against an API configured to call real Claude. The startup command above explicitly selects mock/test mode and clears any inherited API key without reading it. New assertions bound delta payloads and exclude provider diagnostics from the terminal payload. Claude SDK contract tests run real HTTP against private local fixtures in Go, never against Anthropic; they cover forced tools, usage, repairs, retry counts, truncation, refusals and cancellation. No hosted Postman resources or real-key environment files are required.

3C verification on 2026-10-22: the documented command passed 49 requests and 131 assertions, zero failures, exit 0, against the explicitly configured local mock/test API. The four added assertions verify bounded display chunks and the terminal response envelope. SDK protocol/failure behavior was separately exercised through local HTTP fixture tests, with no real Claude request.


## Slice 3D: save and edit workflow

Run the entire maintained collection in order with the mock/test startup and CLI commands above. It captures campaignId, campaignRevision and campaignETag in run-local variables; generated IDs and revision values are never hardcoded. Successful mutations update the token for subsequent edits. The new folder deliberately preserves staleETag for its stale-write regression.

Manual Postman walkthrough:

1. POST /campaigns with a synthetic brief and selected sms/email channels. Capture id and ETag.
2. PUT /campaigns/{{campaignId}}/segment with the existing DSL body and If-Match set to the captured ETag. Capture the returned ETag. A missing token now returns 428; malformed tokens return 400; stale tokens return 412.
3. POST /campaigns/{{campaignId}}/generate-messages without a body. Read the terminal validated event and retain its messages and base_revision together. No templates have been persisted.
4. PUT /campaigns/{{campaignId}}/messages with Content-Type application/json, If-Match constructed as "campaign-<id>-r<base_revision>", and body {"messages":<complete selected-channel message object>}. The server computes current audience bounds and validates before an atomic save. Success is content_ready with a new revision/ETag.
5. GET /campaigns/{{campaignId}}/messages to inspect templates and recorded validation metadata. Edit a field locally, then PUT the entire selected-channel set with that representation's ETag. Every edit is revalidated. Do not automatically retry rejected content with a fresh revision.
6. Try an unknown variable, overlong rendered field, missing channel or duplicate JSON key; GET confirms that text, revision and readiness are unchanged. Repeat an earlier save with staleETag to observe 412.
7. Change or regenerate the segment using the current token. GET messages returns retained human text, draft status and validation=null. PUT that retained/edited set with the current token to revalidate and restore content_ready.

The SSE CLI assertions remain buffered; Go socket tests establish incremental flushing/cancellation. Go PostgreSQL tests, rather than sequential collection requests, establish competing-writer behavior, current mutable audience bounds, cancellation and transactional rollback. Runtime contracts and response examples are canonical in README's Saved messages and editing section. No approval or delivery request is present.

3D verification on 2026-10-22: final local mock/test execution passed 74 requests and 176 assertions, zero failures, exit 0. Cloud reporting was disabled; no real Anthropic or delivery service was called.
