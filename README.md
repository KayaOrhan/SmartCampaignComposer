# Smart Campaign Composer

Slice 1 implements the Go API and PostgreSQL foundation. Requirements: [docs/CASE_STUDY.md](docs/CASE_STUDY.md). Architecture: [DECISIONS.md](DECISIONS.md). Safety: [RULES.md](RULES.md).

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

Configuration uses environment variables only: `DATABASE_URL` (required), `HTTP_ADDR` (default `:8080`), `DB_TIMEOUT` (default `2s`, maximum `30s`). Names appear in `.env.example`. No dotenv loader is used. Later Claude integration must accept `ANTHROPIC_API_KEY` supplied by Compose from user configuration; agents must never inspect that file.

## Tests and checks

Install Go 1.27 and GNU Make; Docker is needed for integration tests.

```sh
make test
make check
make test-integration
```

Unit tests require no database, external API or API key. Initial downloads of pinned modules require registry access. `make check` runs safety/format checks, `go vet` and unit tests. The scanner rejects tracked secret paths without reading their contents and checks safe source files for known key patterns and hidden Unicode; matching values are never printed.

Integration tests create the dedicated disposable `scc_test` database if needed and reset only its test schema/tables. Do not store valuable data there. They verify migration application, repeat execution and transaction rollback. No local filesystem is mounted into the test container.

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

`make check` has not passed here because GNU Make is missing; its Go command equivalents passed. `go test -race` could not run because CGO/compiler support is unavailable. Full assignment functionality is not implemented or verified.

## Known gaps

- Campaigns, Claude, streaming, seed data, mock provider, worker, metrics and Vue are not implemented in Slice 1. Final Compose startup must include seed data and mock provider.
- PostgreSQL remains the queue choice (ADR-006).
- GNU Make is missing from this development PATH; local race testing lacks a CGO compiler.
- Secret scanning detects common formats, not all possible secrets. Claude hooks and session scrubbing remain unverified.
- Current `.gitignore` excludes all `ai-sessions/`; narrow this to raw exports before preparing sanitized submission records.
- Later domain limitations remain in ADR-002/005/013/014: abandonment approximation, SMS segment estimates, final consent race, per-channel ownership and conservative rendering bounds.
- Scheduling, A/B variants, per-user language and CI are optional and deferred.
