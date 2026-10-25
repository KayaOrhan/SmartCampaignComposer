-- Retain campaign content and LLM audit references; no cascading deletion.
ALTER TABLE app.campaigns ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD CONSTRAINT deleted_campaign_unapproved CHECK (
        deleted_at IS NULL OR (status = 'cancelled' AND approved_at IS NULL
            AND approved_revision IS NULL AND approval_fingerprint IS NULL
            AND approval_audience_preview IS NULL));
