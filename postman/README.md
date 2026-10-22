# Local API regression collection

`SmartCampaignComposer.postman_collection.json` is a Postman Collection v2.1 artifact for the implemented Go HTTP API. Import it in Postman or run it with the official Postman CLI (verified with version 1.75.0). No workspace binding, login, API key, custom skill or additional runner dependency is required for this local collection.

From the repository root, start or rebuild the local API:

```powershell
docker compose --env-file .env.example up -d --build api
```

Wait until `http://127.0.0.1:8080/ready` returns HTTP 200. Startup applies migrations and seeds synthetic data once. Then run:

```powershell
postman collection run postman/SmartCampaignComposer.postman_collection.json --env-var "baseUrl=http://127.0.0.1:8080" --bail --timeout-request 5000 --no-report-events
```

The first requests assert liveness and database readiness, so an unavailable API fails the run. The CLI exits nonzero on failed assertions; do not add `--suppress-exit-code`. Run results are kept local with `--no-report-events`. The explicit Compose template avoids automatic loading of `.env`. Docker's Linux engine must be running.

Run the entire collection in order. `baseUrl` defaults to the loopback API; a collection pre-request guard permits only HTTP localhost/127.0.0.1 on port 8080. `campaignId` is captured from each successful create response in run-local scope, overriding stale collection/environment values for subsequent requests. Each full run creates one synthetic draft in the persistent development database. There is no delete endpoint, so the collection does not attempt cleanup or reset database volumes.

Coverage includes all six implemented routes: health, readiness, create/get campaign, save segment and audience preview. Assertions cover canonical channels and segment values, draft persistence, Location, UTC timestamps, JSON/no-store headers, aggregate-only preview fields and union/count bounds. Negative cases check malformed/duplicate JSON, unknown fields, invalid channels/brief/ID, missing campaigns, content type, body size, forbidden consent filters, preview bodies and missing segments. A rejected segment update must preserve the stored segment. The missing-campaign check uses the maximum signed 64-bit ID as an absent fixture, never as a generated resource ID.

Audience counts are deliberately not fixed: seeded events age, and consent/profile edits are preserved. These HTTP assertions do not independently prove consent filtering; PostgreSQL domain tests cover that guarantee. Database-outage 503 and non-draft 409 cases remain covered by Go tests because this API cannot safely create those states through its public endpoints. Generation, approval, delivery and metrics are not implemented and have no requests here. No real LLM or delivery service is called.

The canonical maintenance policy already exists in [AGENTS.md](../AGENTS.md#api-contract--postman-testing). Update this collection alongside API contract changes, using the official Postman plugin workflow. The Go handlers and domain contracts define implemented behavior.

Verification on 2026-10-21: the documented CLI command passed against the local Compose API at `http://127.0.0.1:8080`: 21 requests, 62 assertions, zero failures, exit 0. A temporary copy with a deliberately wrong health-status assertion failed with exit 1; it was removed without changing the maintained collection. `make check` passed (safety/format checks, vet and unit tests). The CLI emitted cloud-login notices but ran locally without login. `postman collection lint` reported `Scanned: 0` for this v2.1 JSON artifact, so it is not counted as schema-validation evidence; execution successfully parsed and ran all requests.
