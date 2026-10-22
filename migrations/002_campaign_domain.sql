CREATE TABLE app.users (
    id BIGINT PRIMARY KEY,
    first_name TEXT NOT NULL CHECK (char_length(first_name) BETWEEN 1 AND 80),
    last_name TEXT NOT NULL CHECK (char_length(last_name) BETWEEN 1 AND 80),
    email TEXT NOT NULL UNIQUE CHECK (char_length(email) BETWEEN 1 AND 254),
    phone TEXT NOT NULL CHECK (char_length(phone) BETWEEN 1 AND 40),
    city TEXT NOT NULL CHECK (char_length(city) BETWEEN 1 AND 120),
    city_norm TEXT NOT NULL CHECK (char_length(city_norm) BETWEEN 1 AND 120),
    language TEXT NOT NULL CHECK (char_length(language) BETWEEN 1 AND 16),
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX users_city_norm_idx ON app.users(city_norm);
CREATE TABLE app.consents (
    user_id BIGINT NOT NULL REFERENCES app.users(id),
    channel TEXT NOT NULL CHECK (channel IN ('sms', 'email')),
    granted BOOLEAN NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, channel)
);
CREATE TABLE app.cart_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES app.users(id),
    product_id BIGINT NOT NULL CHECK (product_id > 0),
    added_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX cart_events_user_time_idx ON app.cart_events(user_id, added_at);
CREATE TABLE app.orders (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES app.users(id),
    placed_at TIMESTAMPTZ NOT NULL,
    total NUMERIC(12,2) NOT NULL CHECK (total >= 0)
);
CREATE INDEX orders_user_time_idx ON app.orders(user_id, placed_at);
CREATE TABLE app.campaigns (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    brief TEXT NOT NULL CHECK (char_length(btrim(brief)) BETWEEN 1 AND 4000),
    channels TEXT[] NOT NULL CHECK (channels IN (ARRAY['sms'], ARRAY['email'], ARRAY['sms','email'])),
    segment_json JSONB CHECK (segment_json IS NULL OR jsonb_typeof(segment_json) = 'object'),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','content_ready','approved','sending','completed','cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at TIMESTAMPTZ
);
CREATE TABLE app.seed_runs (
    version INT PRIMARY KEY,
    reference_time TIMESTAMPTZ NOT NULL
);
