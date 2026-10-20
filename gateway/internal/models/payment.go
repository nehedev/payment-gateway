package models

import (
	"time"
)

type AuthorizePaymentRequest struct {
	OrderID    string `json:"order_id" binding:"required"`
	CustomerID string `json:"customer_id" binding:"required"`
	Amount     int64  `json:"amount" binding:"required,min=1"`
	Currency   string `json:"currency" binding:"required,len=3"`
	CardNumber string `json:"card_number" binding:"required,min=13,max=19"`
	CardExpiry string `json:"card_expiry" binding:"required"` // Format: MM/YYYY
	CardCVV    string `json:"card_cvv" binding:"required,min=3,max=4"`

	// BankIdempotencyKey is the client-supplied Idempotency-Key forwarded to
	// the bank so it can apply its own deduplication. Set by the handler from
	// the HTTP header; not part of the JSON body.
	BankIdempotencyKey string `json:"-"`

	// Gateway-scoped idempotency coordinates, stamped by the idempotency
	// middleware. The service reserves and finalizes the ledger row for
	// (key, path) inside the same transaction that commits the payment
	// record, so the payment outcome and its idempotency entry commit or
	// roll back atomically. Not part of the JSON body.
	GatewayIdempotencyKey      string `json:"-"`
	GatewayIdempotencyPath     string `json:"-"`
	GatewayIdempotencyBodyHash string `json:"-"`
}

// CapturePaymentRequest captures a previously authorized payment.
type CapturePaymentRequest struct {
	// Reference is the payment reference (UUID) returned by the authorize
	// call; taken from the request path, not the JSON body.
	Reference string
	// Amount is the capture amount in cents; the bank requires it to match
	// the authorized amount exactly.
	Amount int64

	// BankIdempotencyKey is the client-supplied Idempotency-Key forwarded to
	// the bank so it can apply its own deduplication on the capture. Not part
	// of the JSON body.
	BankIdempotencyKey string `json:"-"`

	// Gateway-scoped idempotency coordinates — same contract as the authorize
	// request. Not part of the JSON body.
	GatewayIdempotencyKey      string `json:"-"`
	GatewayIdempotencyPath     string `json:"-"`
	GatewayIdempotencyBodyHash string `json:"-"`
}

type PaymentResponse struct {
	PaymentID     string     `json:"payment_id"`
	Reference     string     `json:"reference"`
	OrderID       string     `json:"order_id"`
	CustomerID    string     `json:"customer_id"`
	Amount        int64      `json:"amount"`
	Currency      string     `json:"currency"`
	State         string     `json:"state"`
	BankAuthID    *string    `json:"bank_auth_id,omitempty"`
	BankCaptureID *string    `json:"bank_capture_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	AuthorizedAt  *time.Time `json:"authorized_at,omitempty"`
	CapturedAt    *time.Time `json:"captured_at,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

type CaptureRequest struct {
	Amount     int64  `json:"amount" binding:"required,min=1"`
	BankAuthID string `json:"bank_auth_id" binding:"required"`
}

type CaptureResponse struct {
	Amount        int64      `json:"amount"`
	BankAuthID    string     `json:"bank_auth_id"`
	BankCaptureID string     `json:"bank_capture_id"`
	CapturedAt    *time.Time `json:"captured_at,omitempty"`
	Currency      string     `json:"currency"`
}
