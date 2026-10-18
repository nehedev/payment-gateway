ALTER TABLE idempotency_keys
    ALTER COLUMN response_code SET NOT NULL,
    ALTER COLUMN response_body SET NOT NULL;

ALTER TABLE idempotency_keys DROP COLUMN status;
