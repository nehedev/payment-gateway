-- name: GetIdempotencyKey :one
SELECT id, key, request_path, request_hash, response_code, response_body, status, created_at
FROM idempotency_keys
WHERE key = $1 AND request_path = $2;

-- name: ReserveIdempotencyKey :one
-- Atomically claims an idempotency slot. Returns the inserted row if this
-- request is the first to claim the slot, or ErrNoRows if another request
-- already holds it (concurrent duplicate). The caller must check for ErrNoRows
-- and return 409 Conflict.
INSERT INTO idempotency_keys (key, request_path, request_hash, status)
VALUES ($1, $2, $3, 'in_progress')
ON CONFLICT (key, request_path) DO NOTHING
RETURNING id, key, request_path, request_hash, response_code, response_body, status, created_at;

-- name: FinalizeIdempotencyKey :one
-- Stores the final response on an in_progress record, completing the lifecycle.
UPDATE idempotency_keys
SET status        = 'complete',
    response_code = $3,
    response_body = $4
WHERE key = $1 AND request_path = $2
RETURNING id, key, request_path, request_hash, response_code, response_body, status, created_at;
