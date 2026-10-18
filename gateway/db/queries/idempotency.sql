-- name: GetIdempotencyKey :one
SELECT id, key, request_path, request_hash, response_code, response_body, created_at
FROM idempotency_keys
WHERE key = $1 AND request_path = $2;

-- name: InsertIdempotencyKey :one
INSERT INTO idempotency_keys (key, request_path, request_hash, response_code, response_body)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, key, request_path, request_hash, response_code, response_body, created_at;
