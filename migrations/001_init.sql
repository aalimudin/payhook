CREATE TABLE IF NOT EXISTS events (
    id          BIGSERIAL PRIMARY KEY,
    provider    TEXT NOT NULL,
    event_id    TEXT NOT NULL,
    outcome     TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, event_id)
);

CREATE TABLE IF NOT EXISTS payments (
    provider   TEXT NOT NULL,
    order_ref  TEXT NOT NULL,
    amount     BIGINT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, order_ref)
);

CREATE TABLE IF NOT EXISTS outbox (
    id              BIGSERIAL PRIMARY KEY,
    event_pk        BIGINT NOT NULL REFERENCES events(id),
    idempotency_key TEXT NOT NULL,
    payload         JSONB NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending', -- pending | delivered | dead
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS outbox_due ON outbox (next_attempt_at) WHERE status = 'pending';
