ALTER TABLE app.campaigns
    ADD COLUMN messages_validated_revision BIGINT,
    ADD COLUMN message_validation_preview JSONB,
    ADD CONSTRAINT message_validation_pair CHECK (
        (messages_validated_revision IS NULL) = (message_validation_preview IS NULL)),
    ADD CONSTRAINT message_validation_revision CHECK (
        messages_validated_revision > 0 AND messages_validated_revision <= revision),
    ADD CONSTRAINT message_validation_object CHECK (
        jsonb_typeof(message_validation_preview) = 'object'),
    ADD CONSTRAINT content_ready_validated CHECK (
        status <> 'content_ready' OR (segment_json IS NOT NULL
            AND messages_validated_revision IS NOT NULL
            AND messages_validated_revision = revision
            AND message_validation_preview IS NOT NULL));

CREATE TABLE app.campaign_messages (
    campaign_id BIGINT NOT NULL REFERENCES app.campaigns(id),
    channel TEXT NOT NULL CHECK (channel IN ('sms','email')),
    subject TEXT,
    body TEXT NOT NULL CHECK (char_length(btrim(body)) > 0),
    PRIMARY KEY (campaign_id, channel),
    -- Raw placeholders can be longer than their rendered value. Go enforces
    -- rendered limits against current consent-filtered audience bounds.
    CHECK ((channel = 'sms' AND subject IS NULL AND char_length(body) <= 2240)
        OR (channel = 'email' AND subject IS NOT NULL
            AND char_length(btrim(subject)) BETWEEN 1 AND 1120
            AND char_length(body) <= 28000))
);
