ALTER TABLE app.campaigns
    ADD COLUMN approved_revision BIGINT,
    ADD COLUMN approval_fingerprint TEXT,
    ADD COLUMN approval_audience_preview JSONB,
    ADD CONSTRAINT approval_metadata CHECK (
        (approved_revision IS NULL AND approval_fingerprint IS NULL AND approval_audience_preview IS NULL)
        OR (approved_revision IS NOT NULL AND approved_revision > 0 AND approved_revision < revision
            AND approved_at IS NOT NULL AND approval_fingerprint IS NOT NULL
            AND approval_fingerprint ~ '^[0-9a-f]{64}$'
            AND approval_audience_preview IS NOT NULL
            AND jsonb_typeof(approval_audience_preview) = 'object'
            AND status IN ('approved','sending','completed','cancelled'))),
    ADD CONSTRAINT sending_requires_approval CHECK (
        status NOT IN ('sending','completed') OR approved_revision IS NOT NULL);

CREATE TABLE app.deliveries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES app.campaigns(id),
    user_id BIGINT NOT NULL REFERENCES app.users(id),
    channel TEXT NOT NULL CHECK (channel IN ('sms','email')),
    destination TEXT NOT NULL CHECK (char_length(destination) BETWEEN 1 AND 254),
    subject TEXT,
    body TEXT NOT NULL CHECK (char_length(btrim(body)) > 0),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','in_flight','sent','dead','skipped')),
    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    claim_generation BIGINT NOT NULL DEFAULT 0 CHECK (claim_generation >= 0),
    idempotency_key TEXT NOT NULL UNIQUE,
    provider_message_id TEXT,
    last_error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (campaign_id,user_id,channel),
    CHECK (idempotency_key = 'campaign:' || campaign_id::text || ':user:' || user_id::text || ':' || channel),
    CHECK ((channel = 'sms' AND subject IS NULL AND char_length(body) <= 160)
        OR (channel = 'email' AND subject IS NOT NULL
            AND char_length(btrim(subject)) BETWEEN 1 AND 80 AND char_length(body) <= 2000))
);
CREATE INDEX deliveries_due_idx ON app.deliveries(channel,next_attempt_at,id) WHERE status = 'pending';
