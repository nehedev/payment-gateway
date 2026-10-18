-- Add status column to idempotency_keys to track in-progress vs completed records.
-- This enables the middleware to claim a slot before processing, so concurrent
-- duplicate requests hit the unique constraint rather than both proceeding.

ALTER TABLE idempotency_keys
    ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT 'complete';

-- Make response_code and response_body nullable so an in-progress record can be
-- inserted before the response is known.
ALTER TABLE idempotency_keys
    ALTER COLUMN response_code DROP NOT NULL,
    ALTER COLUMN response_body DROP NOT NULL;

COMMENT ON COLUMN idempotency_keys.status IS 'in_progress: request is being processed; complete: response is cached';
