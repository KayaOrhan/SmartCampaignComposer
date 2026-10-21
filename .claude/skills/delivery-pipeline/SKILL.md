---
name: delivery-pipeline
description: Use when touching the queue, worker, rate limiter, retries, idempotency or the mock provider.
---

# Delivery pipeline

Flavour: Workflow. Sending must be safe to repeat, safe to crash, and never exceed limits.

## Table
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
1. Approve runs in one transaction: `INSERT ... ON CONFLICT DO NOTHING` the audience snapshot, set campaign `sending`. Approving twice changes nothing.
2. One worker loop per channel claims a batch:
   ```sql
   UPDATE deliveries SET status='in_flight', locked_at=now(), attempts=attempts+1
   WHERE id IN (
     SELECT id FROM deliveries
     WHERE channel=$1 AND status='pending' AND next_attempt_at<=now()
     ORDER BY next_attempt_at LIMIT $2
     FOR UPDATE SKIP LOCKED)
   RETURNING *;
   ```
3. Wait for rate-limit capacity with a context-aware paced limiter: no burst credit and at least 10 ms between provider call starts per channel, including retries. Serialize admission, the final consent check and the call start per channel; a delayed consent query must not allow queued calls to bunch up. See ADR-013 for the strict window contract.
4. IMMEDIATELY AFTER the wait, re-check consent with one query, immediately before starting the provider HTTP call with the `Idempotency-Key` header. Missing or revoked consent: `skipped`, provider never called. A consent lookup error fails closed and uses the bounded retry policy; it is never treated as consent. Do not insert another queue or limiter wait between this check and the call. Do not hold a DB transaction open across the HTTP call. Consent changes after this final check cannot be made atomic with an external HTTP request; see ADR-013.
5. 2xx: `sent`. Failure: classify with the retry table below.
6. Reaper: `in_flight` rows older than the visibility timeout go back to `pending`. The resend uses the same idempotency key, so the provider dedupes it.
7. Campaign is `completed` when no `pending` or `in_flight` rows remain.

## Retry policy
- Retry on: network error, timeout, 5xx, 429. For 429 honour `Retry-After` when present.
- Do not retry other 4xx. Mark `dead` immediately.
- Backoff: `min(60s, 1s * 2^attempts)` with full jitter. Maximum 5 attempts, then `dead` with `last_error`.

## Mock provider (`cmd/mockprovider`)
- Faults from env: `FAULT_RATE_5XX`, `FAULT_RATE_429`, `FAULT_LATENCY_P95_MS`, `FAULT_SEED` (seeded, so failures are reproducible).
- Honours `Idempotency-Key`: same key returns the original result and records no second message.
- Enforces its own 100 per second limit with 429 and `Retry-After`. This proves our limiter instead of trusting it.
- `GET /debug/messages` returns unique accepted message counts. Tests assert "unique accepted == sent deliveries".

## Observability
- `/healthz` (process alive), `/readyz` (database and queue reachable), `/metrics`.
- Metrics: `deliveries_total{channel,status}`, `delivery_attempts_total{channel,outcome}`, `queue_depth{channel}`, `provider_latency_seconds{channel}`, `llm_calls_total{call,outcome}`, `llm_validation_failures_total{reason}`.
- Dashboard gets progress over SSE from database counts, not from in-memory counters.

## Tests that must exist
- Two workers in parallel, one killed mid-flight: provider unique count equals delivered count, zero duplicates.
- Limiter with a fake clock: never more than 100 provider call starts in any half-open rolling interval [t, t+1s), including retries. Cover startup, idle recovery, concurrent callers and delayed consent queries; no accumulated burst credit.
- 429 with `Retry-After` is honoured. 4xx goes `dead` at once. Attempt 5 failing goes `dead`.
- Consent revoked between approve and send: `skipped`, provider not called.
- Consent revoked during a limiter wait: final check sees revocation and provider is not called. A failed consent query also prevents the call.
- Reaper recovers a stuck `in_flight` row.

## Gotchas
- The limiter lives in the process. N worker replicas means N times 100 per second. Compose runs one replica per channel. Scaling path (Redis or database token bucket) is in DECISIONS.md.
- Reaper interval must be shorter than the visibility timeout.
- No `time.Sleep` in tests. Inject a clock.

## Provenance
Written 2026-10-21 from the case brief. Add real incidents here with dates.
