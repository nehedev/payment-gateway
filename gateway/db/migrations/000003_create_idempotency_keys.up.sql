CREATE TABLE idempotency_keys (
    id            BIGSERIAL PRIMARY KEY,
    key           VARCHAR(255) NOT NULL,
    request_path  VARCHAR(255) NOT NULL,
    request_hash  VARCHAR(64) NOT NULL,
    response_code INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_idempotency_key_path UNIQUE(key, request_path)
);

CREATE INDEX idx_idempotency_keys_created_at ON idempotency_keys(created_at);

COMMENT ON TABLE idempotency_keys IS 'Stores idempotency keys and cached responses for write operations';
COMMENT ON COLUMN idempotency_keys.key IS 'Client-provided idempotency key from Idempotency-Key header';
COMMENT ON COLUMN idempotency_keys.request_path IS 'Request path to scope idempotency keys';
COMMENT ON COLUMN idempotency_keys.request_hash IS 'SHA256 hash of request body to detect mismatched retries';
COMMENT ON COLUMN idempotency_keys.response_code IS 'HTTP status code of the original response';
COMMENT ON COLUMN idempotency_keys.response_body IS 'Original JSON response body to replay';
