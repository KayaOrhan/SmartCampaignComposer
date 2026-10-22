---
name: delivery-pipeline
description: Use when touching the queue, worker, rate limiter, retries, idempotency or the mock provider.
---

# Delivery pipeline

Flavour: Workflow. Sending must be safe to repeat, safe to crash, and never exceed limits.

## Table
Migrations 007/008 are the authoritative implemented schema. Implemented rows freeze destination, subject and body, carry lease_expires_at and claim_generation, enforce a unique deterministic idempotency_key and use last_error_code (safe codes only), rather than raw last_error. Migration 008 also stores shared channel admission and durable mock acceptance/attempt/rate state. No private recipient payload may enter HTTP status responses or logs. The sketch below is explanatory; do not copy it over the authoritative migrations.

```sql
CREATE TABLE deliveries (
  id BIGSERIAL PRIMARY KEY,
  campaign_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  channel TEXT NOT NULL CHECK (channel IN ('sms','email')),
  status TEXT NOT NULL CHECK (status IN ('pending','in_flight','sent','dead','skipped')),
  attempts INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_at TIMESTAMPTZ,
  idempotency_key TEXT NOT NULL,
  provider_message_id TEXT,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (campaign_id, user_id, channel)
);
```
`idempotency_key` is deterministic: `campaign:{id}:user:{id}:{channel}`. `skipped` means consent was revoked before sending.

## Flow
1. Approve runs in one transaction: lock/compare campaign revision, revalidate saved templates against the current consent-aware audience, compare the reviewed fingerprint, insert the frozen audience/payload snapshot and set campaign sending. Unexpected pre-existing rows fail closed. A reconciled repeat using the current revision and original fingerprint changes nothing; an old revision still fails. README and ADR-023/024 define the implemented API and worker behavior.
2. Configurable goroutines per channel claim one row at a time. The implemented query also checks campaign state/attempt limit and advances generation/lease; the locking pattern is:
   ```sql
   UPDATE deliveries SET status='in_flight', locked_at=now(), attempts=attempts+1
   WHERE id IN (
     SELECT id FROM deliveries
     WHERE channel=$1 AND status='pending' AND next_attempt_at<=now()
     ORDER BY next_attempt_at LIMIT $2
     FOR UPDATE SKIP LOCKED)
   RETURNING *;
   ```
3. Wait for shared channel admission using a session advisory lock and PostgreSQL next-start state; renew claims waiting for admission. No burst credit: normal spacing is response completion plus 10 ms, including retries. Reserve admission through the lease before dispatch so process crashes leave a cooldown. Serialize admission, final consent and HTTP per channel across processes. See ADR-024, superseding ADR-013's single-replica assumption. No database transaction spans HTTP.
4. IMMEDIATELY AFTER the wait, re-check consent with one query, immediately before starting the provider HTTP call with the `Idempotency-Key` header. Missing or revoked consent: `skipped`, provider never called. A consent lookup error fails closed and uses the bounded retry policy; it is never treated as consent. Do not insert another queue or limiter wait between this check and the call. Do not hold a DB transaction open across the HTTP call. Consent changes after this final check cannot be made atomic with an external HTTP request; see ADR-013.
5. 2xx: `sent`. Failure: classify with the retry table below.
6. Reaper: expired in_flight leases go back to pending, or dead after five processing claims. Fence dispatch/results by generation and unexpired lease. The resend uses the same idempotency key; the provider's PostgreSQL acceptance ledger deduplicates across restarts. An accepted final attempt without an acknowledgement may still become dead; never claim exact outcome reconciliation.
7. Campaign is completed when no pending/in_flight rows remain. Result/recovery transactions lock campaign before delivery; progress never increments campaign revision.

## Retry policy
- Retry on: network error, timeout, 5xx, 429. For 429 honour `Retry-After` when present.
- Do not retry other 4xx. Mark `dead` immediately.
- Backoff: `min(60s, 1s * 2^attempts)` with full jitter. Maximum five processing claims (including abandoned ones), then dead with safe last_error_code. Expired leases are immediately due when below that limit.

## Mock provider (`cmd/mockprovider`)
- Faults from env: `FAULT_RATE_5XX`, `FAULT_RATE_429`, `FAULT_LATENCY_P95_MS`, `FAULT_SEED` (seeded, so failures are reproducible).
- Honours Idempotency-Key: an accepted matching key returns the original receipt and records no second acceptance, even after restart. Payload hashes reject conflicting content; no recipient payload is stored in the mock ledger.
- Enforces its own 100 per second limit with 429 and `Retry-After`. This proves our limiter instead of trusting it.
- `GET /debug/messages` returns unique accepted message counts. Tests assert "unique accepted == sent deliveries".

## Observability
- Implemented API/mock provider endpoints are /health and /ready (ADR-015). GET delivery-status reads PostgreSQL counters; mock GET /debug/messages exposes aggregate accepted counts. Metrics endpoints and dashboard push remain planned.
- Metrics: `deliveries_total{channel,status}`, `delivery_attempts_total{channel,outcome}`, `queue_depth{channel}`, `provider_latency_seconds{channel}`, `llm_calls_total{call,outcome}`, `llm_validation_failures_total{reason}`.
- Dashboard gets progress over SSE from database counts, not from in-memory counters.

## Tests that must exist
- Two workers in parallel, one killed mid-flight: provider unique count equals delivered count, zero duplicates.
- Database-authoritative admission tests measure rolling windows with PostgreSQL timestamps, including concurrent/repeated dispatch and shared admission. Test waiting-lease renewal and cancellation. Never exceed 100 starts in any half-open rolling interval [t, t+1s), including retries; no accumulated burst credit. Pure retry policy tests inject jitter; do not substitute a fake process clock for the PostgreSQL gate.
- 429 with `Retry-After` is honoured. 4xx goes `dead` at once. Attempt 5 failing goes `dead`.
- Consent revoked between approve and send: `skipped`, provider not called.
- Consent revoked during a limiter wait: final check sees revocation and provider is not called. A failed consent query also prevents the call.
- Reaper recovers a stuck `in_flight` row.

## Gotchas
- Admission is shared in PostgreSQL. More replicas cannot multiply the configured per-channel rate. Session advisory locks need direct/session-preserving connections; transaction-pooling proxies are unsupported.
- Worker polling recovers expired leases. Provider timeout plus database deadlines must fit within the lease. Keep worker/database clocks synchronized for absolute call deadlines.
- No `time.Sleep` in tests. Inject a clock.

## Provenance
Written 2026-10-21 from the case brief. Add real incidents here with dates.
