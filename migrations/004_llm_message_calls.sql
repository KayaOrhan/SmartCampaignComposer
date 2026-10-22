ALTER TABLE app.llm_calls DROP CONSTRAINT llm_calls_call_name_check;
ALTER TABLE app.llm_calls ADD CONSTRAINT llm_calls_call_name_check
    CHECK (call_name IN ('ExtractSegment', 'GenerateMessages'));
