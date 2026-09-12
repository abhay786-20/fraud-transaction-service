CREATE TABLE transactions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sender_id     UUID NOT NULL,
    receiver_id   UUID NOT NULL,
    amount        NUMERIC(18, 2) NOT NULL CHECK (amount > 0),
    currency      VARCHAR(3) NOT NULL DEFAULT 'INR',
    status        VARCHAR(20) NOT NULL DEFAULT 'completed'
                  CHECK (status IN ('pending', 'completed', 'failed', 'flagged')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id   UUID NOT NULL,
    event_type     VARCHAR(50) NOT NULL,
    payload        JSONB NOT NULL,
    published_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Justified from day one, unlike fraud-auth-service's email/role indexes
-- — we already know the exact, permanent query pattern: "find unpublished
-- events, oldest first." Partial index: only rows still unpublished are
-- indexed at all, so it stays small forever instead of growing with the
-- table.
CREATE INDEX idx_outbox_events_unpublished ON outbox_events (created_at)
    WHERE published_at IS NULL;
