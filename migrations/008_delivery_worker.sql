CREATE INDEX deliveries_expired_idx ON app.deliveries(lease_expires_at,id) WHERE status='in_flight';
ALTER TABLE app.deliveries ADD CONSTRAINT deliveries_attempt_limit CHECK(attempts<=5);
CREATE TABLE app.delivery_channels (
    channel TEXT PRIMARY KEY CHECK (channel IN ('sms','email')),
    next_start_at TIMESTAMPTZ NOT NULL DEFAULT '-infinity'
);
INSERT INTO app.delivery_channels(channel) VALUES ('sms'),('email');
-- This mock-only acceptance ledger deliberately stores no recipient payload.
CREATE TABLE app.mock_acceptances (
    idempotency_key TEXT PRIMARY KEY,
    payload_hash TEXT NOT NULL,
    channel TEXT NOT NULL CHECK (channel IN ('sms','email')),
    message_id TEXT NOT NULL UNIQUE,
    accepted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX mock_acceptances_rate_idx ON app.mock_acceptances(channel,accepted_at);
CREATE TABLE app.mock_attempts (
    idempotency_key TEXT PRIMARY KEY,
    attempts INT NOT NULL CHECK (attempts>0)
);
CREATE TABLE app.mock_calls (
    channel TEXT NOT NULL CHECK (channel IN ('sms','email')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX mock_calls_window_idx ON app.mock_calls(channel,started_at);
