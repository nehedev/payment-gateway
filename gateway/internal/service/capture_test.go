package service

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/nehemiah-dev/payment-gateway/internal/bankclient"
	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

func TestCaptureStateCheckRejectsUnauthorizedPaymentState(t *testing.T) {
	req := &models.CapturePaymentRequest{Reference: "ref-1", Amount: 5000}
	payment := &store.Payment{PaymentState: "pending", AmountCents: 5000}

	stateErr := captureStateCheck(req, payment)

	require.NotNil(t, stateErr)
	require.Equal(t, http.StatusConflict, stateErr.StatusCode)
	require.Equal(t, "payment_not_authorized", stateErr.Code)
}

func TestCaptureStateCheckRejectsCapturedPaymentState(t *testing.T) {
	req := &models.CapturePaymentRequest{Reference: "ref-1", Amount: 5000}
	payment := &store.Payment{
		PaymentState: "captured",
		AmountCents:  5000,
		CaptureID:    pgtype.Text{String: "cap_x", Valid: true},
	}

	stateErr := captureStateCheck(req, payment)

	require.NotNil(t, stateErr)
	require.Equal(t, http.StatusConflict, stateErr.StatusCode)
	require.Equal(t, "payment_not_authorized", stateErr.Code)
}

func TestCaptureStateCheckAcceptsAuthorizedPayment(t *testing.T) {
	req := &models.CapturePaymentRequest{Reference: "ref-1", Amount: 5000}
	payment := &store.Payment{
		PaymentState:    "authorized",
		AmountCents:     5000,
		AuthorizationID: pgtype.Text{String: "auth_x", Valid: true},
	}

	require.Nil(t, captureStateCheck(req, payment))
}

func TestCaptureStateCheckRejectsAmountMismatch(t *testing.T) {
	req := &models.CapturePaymentRequest{Reference: "ref-1", Amount: 1000}
	payment := &store.Payment{PaymentState: "authorized", AmountCents: 5000}

	stateErr := captureStateCheck(req, payment)

	require.NotNil(t, stateErr)
	require.Equal(t, http.StatusBadRequest, stateErr.StatusCode)
	require.Equal(t, "amount_mismatch", stateErr.Code)
}

func TestAuthUnusableDeclinesFailThePayment(t *testing.T) {
	unusable := []bankclient.ErrorCode{
		bankclient.ErrorAuthorizationNotFound,
		bankclient.ErrorAuthorizationExpired,
		bankclient.ErrorAuthorizationAlreadyUsed,
		bankclient.ErrorAlreadyCaptured,
	}
	for _, code := range unusable {
		require.True(t, authUnusable(&bankclient.Error{Code: code}),
			"expected %s to mark the authorization unusable", code)
	}
}

func TestAuthUnusableKeepsClientFixableDeclines(t *testing.T) {
	fixable := []bankclient.ErrorCode{
		bankclient.ErrorAmountMismatch,
		bankclient.ErrorInvalidAmount,
	}
	for _, code := range fixable {
		require.False(t, authUnusable(&bankclient.Error{Code: code}),
			"expected %s to leave the authorization intact", code)
	}

	// Transport errors are unknown outcomes, not unusable authorizations.
	require.False(t, authUnusable(&bankclient.Error{}))
	require.False(t, authUnusable(nil))
}

func TestValidateCaptureRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &models.CapturePaymentRequest{
			Reference: "550e8400-e29b-41d4-a716-446655440000",
			Amount:    5000,
		}
		require.NoError(t, ValidateCaptureRequest(req))
	})

	t.Run("zero amount", func(t *testing.T) {
		req := &models.CapturePaymentRequest{
			Reference: "550e8400-e29b-41d4-a716-446655440000",
			Amount:    0,
		}
		require.Error(t, ValidateCaptureRequest(req))
	})

	t.Run("non-uuid reference", func(t *testing.T) {
		req := &models.CapturePaymentRequest{Reference: "not-a-uuid", Amount: 100}
		require.Error(t, ValidateCaptureRequest(req))
	})
}
