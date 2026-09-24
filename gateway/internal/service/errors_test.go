package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nehemiah-dev/payment-gateway/internal/bankclient"
)

func TestMapBankError(t *testing.T) {
	tests := []struct {
		name           string
		bankErr        error
		wantStatusCode int
		wantCode       string
	}{
		{
			name: "insufficient funds",
			bankErr: &bankclient.Error{
				StatusCode: 402,
				Code:       bankclient.ErrorInsufficientFunds,
				Message:    "Insufficient funds",
				Retryable:  false,
			},
			wantStatusCode: http.StatusPaymentRequired,
			wantCode:       "insufficient_funds",
		},
		{
			name: "card expired",
			bankErr: &bankclient.Error{
				StatusCode: 400,
				Code:       bankclient.ErrorCardExpired,
				Message:    "Card has expired",
				Retryable:  false,
			},
			wantStatusCode: http.StatusBadRequest,
			wantCode:       "card_expired",
		},
		{
			name: "invalid card",
			bankErr: &bankclient.Error{
				StatusCode: 400,
				Code:       bankclient.ErrorInvalidCard,
				Message:    "Invalid card number",
				Retryable:  false,
			},
			wantStatusCode: http.StatusBadRequest,
			wantCode:       "invalid_card",
		},
		{
			name: "invalid CVV",
			bankErr: &bankclient.Error{
				StatusCode: 400,
				Code:       bankclient.ErrorInvalidCVV,
				Message:    "Invalid CVV",
				Retryable:  false,
			},
			wantStatusCode: http.StatusBadRequest,
			wantCode:       "invalid_card",
		},
		{
			name: "invalid amount",
			bankErr: &bankclient.Error{
				StatusCode: 400,
				Code:       bankclient.ErrorInvalidAmount,
				Message:    "Amount must be positive",
				Retryable:  false,
			},
			wantStatusCode: http.StatusBadRequest,
			wantCode:       "validation_error",
		},
		{
			name: "bank internal error - retryable",
			bankErr: &bankclient.Error{
				StatusCode: 500,
				Code:       bankclient.ErrorInternal,
				Message:    "Internal server error",
				Retryable:  true,
			},
			wantStatusCode: http.StatusServiceUnavailable,
			wantCode:       "bank_unavailable",
		},
		{
			name: "bank internal error - not retryable",
			bankErr: &bankclient.Error{
				StatusCode: 500,
				Code:       bankclient.ErrorInternal,
				Message:    "Internal server error",
				Retryable:  false,
			},
			wantStatusCode: http.StatusInternalServerError,
			wantCode:       "internal_error",
		},
		{
			name: "unknown bank error - retryable",
			bankErr: &bankclient.Error{
				StatusCode: 503,
				Code:       "unknown_error",
				Message:    "Unknown error",
				Retryable:  true,
			},
			wantStatusCode: http.StatusServiceUnavailable,
			wantCode:       "bank_unavailable",
		},
		{
			name: "unknown bank error - not retryable",
			bankErr: &bankclient.Error{
				StatusCode: 400,
				Code:       "unknown_error",
				Message:    "Unknown error",
				Retryable:  false,
			},
			wantStatusCode: http.StatusBadRequest,
			wantCode:       "unknown_error",
		},
		{
			name:           "non-bank error",
			bankErr:        errors.New("some random error"),
			wantStatusCode: http.StatusInternalServerError,
			wantCode:       "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MapBankError(tt.bankErr)

			var serviceErr *ServiceError
			assert.True(t, errors.As(err, &serviceErr), "expected ServiceError")
			assert.Equal(t, tt.wantStatusCode, serviceErr.StatusCode)
			assert.Equal(t, tt.wantCode, serviceErr.Code)
		})
	}
}

func TestServiceError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *ServiceError
		wantMsg string
	}{
		{
			name: "with wrapped error",
			err: &ServiceError{
				StatusCode: 400,
				Code:       "validation_error",
				Message:    "Invalid input",
				Err:        errors.New("underlying error"),
			},
			wantMsg: "Invalid input: underlying error",
		},
		{
			name: "without wrapped error",
			err: &ServiceError{
				StatusCode: 400,
				Code:       "validation_error",
				Message:    "Invalid input",
			},
			wantMsg: "Invalid input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMsg, tt.err.Error())
		})
	}
}

func TestServiceError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &ServiceError{
		StatusCode: 500,
		Code:       "internal_error",
		Message:    "Something went wrong",
		Err:        underlying,
	}

	assert.Equal(t, underlying, errors.Unwrap(err))
}
