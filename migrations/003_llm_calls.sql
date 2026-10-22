CREATE TABLE app.llm_calls (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    campaign_id BIGINT NOT NULL REFERENCES app.campaigns(id),
    call_name TEXT NOT NULL CHECK (call_name = 'ExtractSegment'),
    prompt_version TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('success','llm_output_invalid','llm_timeout','llm_unavailable','llm_refused')),
    validation_errors JSONB NOT NULL CHECK (jsonb_typeof(validation_errors) = 'array'),
    latency_ms BIGINT NOT NULL CHECK (latency_ms >= 0),
    input_tokens INT NOT NULL CHECK (input_tokens >= 0),
    output_tokens INT NOT NULL CHECK (output_tokens >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
