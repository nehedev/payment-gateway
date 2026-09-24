package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/nehemiah-dev/payment-gateway/internal/bankclient"
)

// PaymentError represents an error from the payment service layer.
type PaymentError struct {
	StatusCode int
	Code       string
	Message    string
	Err        error
}

func (e *PaymentError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *PaymentError) Unwrap() error {
	return e.Err
}

// Error constructors

func NewValidationError(message string) *PaymentError {
	return &PaymentError{
		StatusCode: http.StatusBadRequest,
		Code:       "validation_error",
		Message:    message,
	}
}

func NewInsufficientFundsError(message string) *PaymentError {
	return &PaymentError{
		StatusCode: http.StatusPaymentRequired,
		Code:       "insufficient_funds",
		Message:    message,
	}
}

func NewCardExpiredError(message string) *PaymentError {
	return &PaymentError{
		StatusCode: http.StatusBadRequest,
		Code:       "card_expired",
		Message:    message,
	}
}

func NewInvalidCardError(message string) *PaymentError {
	return &PaymentError{
		StatusCode: http.StatusBadRequest,
		Code:       "invalid_card",
		Message:    message,
	}
}

func NewBankUnavailableError(err error) *PaymentError {
	return &PaymentError{
		StatusCode: http.StatusServiceUnavailable,
		Code:       "bank_unavailable",
		Message:    "Payment processor temporarily unavailable, please try again",
		Err:        err,
	}
}

func NewInternalError(err error) *PaymentError {
	return &PaymentError{
		StatusCode: http.StatusInternalServerError,
		Code:       "internal_error",
		Message:    "Internal server error",
		Err:        err,
	}
}

// MapBankError converts bank client errors to service errors with appropriate HTTP status codes.
func MapBankError(err error) error {
	var bankErr *bankclient.Error
	if !errors.As(err, &bankErr) {
		// Not a bank error - treat as internal error
		return NewInternalError(err)
	}

	switch bankErr.Code {
	case bankclient.ErrorInsufficientFunds:
		return NewInsufficientFundsError(bankErr.Message)

	case bankclient.ErrorCardExpired:
		return NewCardExpiredError(bankErr.Message)

	case bankclient.ErrorInvalidCard,
		bankclient.ErrorInvalidCVV:
		return NewInvalidCardError(bankErr.Message)

	case bankclient.ErrorInvalidAmount:
		return NewValidationError(bankErr.Message)

	case bankclient.ErrorInternal:
		if bankErr.Retryable {
			return NewBankUnavailableError(err)
		}
		return NewInternalError(err)

	default:
		// Unknown bank error
		if bankErr.Retryable {
			return NewBankUnavailableError(err)
		}
		return &PaymentError{
			StatusCode: http.StatusBadRequest,
			Code:       string(bankErr.Code),
			Message:    bankErr.Message,
			Err:        err,
		}
	}
}
