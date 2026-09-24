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
}

type PaymentResponse struct {
	PaymentID    string     `json:"payment_id"`
	Reference    string     `json:"reference"`
	OrderID      string     `json:"order_id"`
	CustomerID   string     `json:"customer_id"`
	Amount       int64      `json:"amount"`
	Currency     string     `json:"currency"`
	State        string     `json:"state"`
	BankAuthID   *string    `json:"bank_auth_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	AuthorizedAt *time.Time `json:"authorized_at,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
