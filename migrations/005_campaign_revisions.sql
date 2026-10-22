ALTER TABLE app.campaigns ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0);
