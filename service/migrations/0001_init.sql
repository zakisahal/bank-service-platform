CREATE TABLE IF NOT EXISTS accounts (
    id            TEXT PRIMARY KEY,
    owner         TEXT NOT NULL,
    balance_cents BIGINT NOT NULL DEFAULT 0,
    currency      TEXT NOT NULL DEFAULT 'USD',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO accounts (id, owner, balance_cents, currency) VALUES
    ('acc_1001', 'Ada Lovelace', 542300, 'USD'),
    ('acc_1002', 'Grace Hopper', 1287650, 'USD')
ON CONFLICT (id) DO NOTHING;
