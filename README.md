# Smart Campaign Composer

Slices 1 and 2 implement the Go/PostgreSQL foundation, campaign drafts, fake seed data and validated audience previews. Requirements: [docs/CASE_STUDY.md](docs/CASE_STUDY.md). Architecture: [DECISIONS.md](DECISIONS.md). Safety: [RULES.md](RULES.md).

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
| `POST /campaigns/{id}/audience-preview` | No body | 200 aggregate preview |

Briefs must contain 1 to 4000 runes after trimming. Channels are a nonempty unique selection of `sms`/`email`, stored in canonical order. Segments can only be changed on drafts. There is no generation, approval or send endpoint yet. Preview without a segment returns 409.

Example segment:

```json
{"version":1,"all":[{"field":"city","op":"in","value":["istanbul"]},{"field":"cart_added","op":"within_days","value":30},{"field":"purchase","op":"none_within_days","value":30}]}
```

The [campaign-domain skill](.claude/skills/campaign-domain/SKILL.md) specifies allowed fields, operators, limits, normalization and UTC window boundaries. Consent is always added by the compiler and cannot be supplied or removed by a segment. All values become bound SQL parameters; segment text is never executed.

Preview returns `as_of`, `eligible_unique` (the union across selected channels), and `by_channel` with `count`, `max_first_name_runes` and `max_city_runes`. Both maxima are independent, even when different recipients supply them. No recipient rows or contact details are returned. A repeatable-read transaction keeps channel counts consistent; this is not an approval snapshot.

JSON requests require Content-Type `application/json`. Validation rejects duplicate or unknown keys, wrong types, invalid UTF-8 and trailing values. Error responses have the form `{"error":{"code":"unsupported_field","path":"all[0].field"}}`, without echoing submitted values or driver errors. Statuses: 400 malformed JSON, 413 oversized body, 422 invalid contract, 404 missing campaign, 409 invalid state/missing segment, 503 database unavailable.

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

- Claude, streaming, campaign messages/approval, mock provider, worker, metrics and Vue are not implemented yet. Final Compose startup must also include the mock provider.
- PostgreSQL remains the queue choice (ADR-006).
- Local race testing lacks a CGO compiler.
- Secret scanning detects common formats, not all possible secrets. Claude hooks and session scrubbing remain unverified.
- Current `.gitignore` excludes all `ai-sessions/`; narrow this to raw exports before preparing sanitized submission records.
- Later domain limitations remain in ADR-002/005/013/014: abandonment approximation, SMS segment estimates, final consent race, per-channel ownership and conservative rendering bounds.
- Scheduling, A/B variants, per-user language and CI are optional and deferred.
