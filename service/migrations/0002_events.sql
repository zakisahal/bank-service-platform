-- Dedupe table for the event consumer: the presence of a row is the only
-- fact that matters. event_id is the domain-level idempotency key assigned
-- by the producer (stable across redeliveries), independent of the Redis
-- Stream entry ID, which changes per delivery attempt and can't be used for
-- dedup.
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The side effect the consumer performs: one durable audit row per event,
-- written in the same transaction as the processed_events insert so the two
-- can never disagree (see internal/events/consumer.go).
CREATE TABLE IF NOT EXISTS audit_log (
    id          BIGSERIAL PRIMARY KEY,
    event_id    TEXT NOT NULL REFERENCES processed_events (event_id),
    account_id  TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    payload     JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_log_account_id_idx ON audit_log (account_id);
