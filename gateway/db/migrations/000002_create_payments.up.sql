CREATE TABLE payments (
    id                BIGSERIAL PRIMARY KEY,
    reference         UUID NOT NULL DEFAULT gen_random_uuid(),
    amount_cents      BIGINT NOT NULL CHECK (amount_cents > 0),
    currency          VARCHAR(3) NOT NULL DEFAULT 'USD' CHECK (currency ~ '^[A-Z]{3}$'),
    card_last_four    VARCHAR(4) NOT NULL CHECK (card_last_four ~ '^[0-9]{4}$'),
    customer_id       VARCHAR(50) NOT NULL,
    order_id          VARCHAR(50) NOT NULL,
    payment_state     VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (payment_state IN ('pending', 'authorized', 'captured', 'voided', 'refunded', 'failed')),
    authorization_id  VARCHAR(50),
    capture_id        VARCHAR(50),
    refund_id         VARCHAR(50),
    void_id           VARCHAR(50),
    metadata          JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_payments_reference ON payments(reference);
CREATE INDEX idx_payments_order_id ON payments(order_id);
CREATE INDEX idx_payments_customer_id ON payments(customer_id);

CREATE UNIQUE INDEX idx_payments_authorization_id ON payments(authorization_id);
CREATE UNIQUE INDEX idx_payments_capture_id ON payments(capture_id);
CREATE UNIQUE INDEX idx_payments_refund_id ON payments(refund_id);
CREATE UNIQUE INDEX idx_payments_void_id ON payments(void_id);

CREATE TRIGGER trg_payments_updated_at
    BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();