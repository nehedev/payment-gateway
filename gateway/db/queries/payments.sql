-- name: CreatePayment :one
INSERT INTO payments (
    amount_cents, currency, card_last_four, customer_id, order_id, metadata
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetPaymentByID :one
SELECT * FROM payments WHERE id = $1;

-- name: GetPaymentByReference :one
SELECT * FROM payments WHERE reference = $1;

-- name: ListPaymentsByCustomer :many
SELECT * FROM payments
WHERE customer_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListPaymentsByOrder :many
SELECT * FROM payments
WHERE order_id = $1
ORDER BY created_at DESC;

-- name: AuthorizePayment :one
UPDATE payments
SET payment_state = 'authorized',
    authorization_id = $2,
    updated_at = now()
WHERE id = $1 AND payment_state = 'pending'
RETURNING *;

-- name: CapturePayment :one
UPDATE payments
SET payment_state = 'captured',
    capture_id = $2,
    updated_at = now()
WHERE id = $1 AND payment_state = 'authorized'
RETURNING *;

-- name: VoidPayment :one
UPDATE payments
SET payment_state = 'voided',
    void_id = $2,
    updated_at = now()
WHERE id = $1 AND payment_state = 'authorized'
RETURNING *;

-- name: RefundPayment :one
UPDATE payments
SET payment_state = 'refunded',
    refund_id = $2,
    updated_at = now()
WHERE id = $1 AND payment_state = 'captured'
RETURNING *;

-- name: FailPayment :one
UPDATE payments
SET payment_state = 'failed',
    updated_at = now()
WHERE id = $1 AND payment_state IN ('pending', 'authorized')
RETURNING *;