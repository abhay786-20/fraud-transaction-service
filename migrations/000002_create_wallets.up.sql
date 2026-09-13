CREATE TABLE wallets (
    user_id    UUID PRIMARY KEY,
    balance    NUMERIC(18, 2) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
