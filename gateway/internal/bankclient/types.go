package bankclient

import "time"

// ErrorCode is the bank's error taxonomy, typed so callers can switch on
// specific outcomes (e.g. "already captured" vs "insufficient funds")
// instead of string-matching a message.
type ErrorCode string

const (
	ErrorInvalidCard              ErrorCode = "invalid_card"
	ErrorInvalidCVV               ErrorCode = "invalid_cvv"
	ErrorInvalidAmount            ErrorCode = "invalid_amount"
	ErrorCardExpired              ErrorCode = "card_expired"
	ErrorInsufficientFunds        ErrorCode = "insufficient_funds"
	ErrorMissingIdempotencyKey    ErrorCode = "missing_idempotency_key"
	ErrorAuthorizationNotFound    ErrorCode = "authorization_not_found"
	ErrorAuthorizationExpired     ErrorCode = "authorization_expired"
	ErrorAuthorizationAlreadyUsed ErrorCode = "authorization_already_used"
	ErrorAlreadyCaptured          ErrorCode = "already_captured"
	ErrorAlreadyVoided            ErrorCode = "already_voided"
	ErrorAlreadyRefunded          ErrorCode = "already_refunded"
	ErrorAmountMismatch           ErrorCode = "amount_mismatch"
	ErrorCaptureNotFound          ErrorCode = "capture_not_found"
	ErrorRefundNotFound           ErrorCode = "refund_not_found"
	ErrorNotFound                 ErrorCode = "not_found"
	ErrorInternal                 ErrorCode = "internal_error"
)

// --- Domain-facing types (what the service layer uses) ---
// Kept separate from the wire structs below so the service layer never
// depends on the bank's exact JSON field names — same reasoning as
// database.PoolConfig staying decoupled from config.DBConfig earlier.

type AuthorizeRequest struct {
	IdempotencyKey string
	CardNumber     string // raw PAN — see PCI note; never log or persist this
	CVV            string
	ExpiryMonth    int
	ExpiryYear     int
	AmountCents    int64
}

type AuthorizeResult struct {
	AuthorizationID string
	Status          string
	AmountCents     int64
	Currency        string
	ExpiresAt       time.Time
	CreatedAt       time.Time
}

type CaptureRequest struct {
	IdempotencyKey  string
	AuthorizationID string
	AmountCents     int64
}

type CaptureResult struct {
	CaptureID       string
	AuthorizationID string
	Status          string
	AmountCents     int64
	Currency        string
	CapturedAt      time.Time
}

type VoidRequest struct {
	IdempotencyKey  string
	AuthorizationID string
}

type VoidResult struct {
	VoidID          string
	AuthorizationID string
	Status          string
	VoidedAt        time.Time
}

type RefundRequest struct {
	IdempotencyKey string
	CaptureID      string
	AmountCents    int64
}

type RefundResult struct {
	RefundID    string
	CaptureID   string
	Status      string
	AmountCents int64
	Currency    string
	RefundedAt  time.Time
}

// --- Wire types (unexported — exact shape of the bank's JSON) ---

type authorizeWireRequest struct {
	CardNumber  string `json:"card_number"`
	CVV         string `json:"cvv"`
	ExpiryMonth int    `json:"expiry_month"`
	ExpiryYear  int    `json:"expiry_year"`
	Amount      int64  `json:"amount"`
}

type authorizationWireResponse struct {
	AuthorizationID string    `json:"authorization_id"`
	Status          string    `json:"status"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type captureWireRequest struct {
	AuthorizationID string `json:"authorization_id"`
	Amount          int64  `json:"amount"`
}

type captureWireResponse struct {
	CaptureID       string    `json:"capture_id"`
	AuthorizationID string    `json:"authorization_id"`
	Status          string    `json:"status"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	CapturedAt      time.Time `json:"captured_at"`
}

type voidWireRequest struct {
	AuthorizationID string `json:"authorization_id"`
}

type voidWireResponse struct {
	VoidID          string    `json:"void_id"`
	AuthorizationID string    `json:"authorization_id"`
	Status          string    `json:"status"`
	VoidedAt        time.Time `json:"voided_at"`
}

type refundWireRequest struct {
	CaptureID string `json:"capture_id"`
	Amount    int64  `json:"amount"`
}

type refundWireResponse struct {
	RefundID   string    `json:"refund_id"`
	CaptureID  string    `json:"capture_id"`
	Status     string    `json:"status"`
	Amount     int64     `json:"amount"`
	Currency   string    `json:"currency"`
	RefundedAt time.Time `json:"refunded_at"`
}
