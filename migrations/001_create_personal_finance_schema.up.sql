CREATE TABLE accounts (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('checking', 'savings', 'credit_card', 'cash', 'investment')),
    currency    CHAR(3) NOT NULL DEFAULT 'USD',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE TABLE categories (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL CHECK (kind IN ('income', 'expense')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE transactions (
    id           BIGSERIAL PRIMARY KEY,
    account_id   BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    category_id  BIGINT REFERENCES categories(id) ON DELETE SET NULL,
    amount_cents BIGINT NOT NULL,  -- positive = money in, negative = money out
    description  TEXT,
    occurred_on  DATE NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_transactions_account_date ON transactions (account_id, occurred_on DESC);
CREATE INDEX idx_transactions_category     ON transactions (category_id);
CREATE INDEX idx_transactions_date         ON transactions (occurred_on DESC);
