package service

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

func TestRefundStateCheckRejectsNonCapturedPaymentState(t *testing.T) {
	states := []string{"pending", "authorized", "voided", "refunded", "failed"}
	for _, state := range states {
		req := &models.RefundPaymentRequest{Reference: "ref-1", Amount: 5000}
		payment := &store.Payment{
			PaymentState: state,
			AmountCents:  5000,
			CaptureID:    pgtype.Text{String: "cap_x", Valid: true},
		}

		stateErr := refundStateCheck(req, payment)

		require.NotNil(t, stateErr, "state %s should be non-refundable", state)
		require.Equal(t, http.StatusConflict, stateErr.StatusCode)
		require.Equal(t, "payment_not_captured", stateErr.Code)
	}
}

func TestRefundStateCheckAcceptsCapturedPayment(t *testing.T) {
	req := &models.RefundPaymentRequest{Reference: "ref-1", Amount: 5000}
	payment := &store.Payment{
		PaymentState: "captured",
		AmountCents:  5000,
		CaptureID:    pgtype.Text{String: "cap_550e8400", Valid: true},
	}

	require.Nil(t, refundStateCheck(req, payment))
}

func TestRefundStateCheckRejectsMissingRecordedCapture(t *testing.T) {
	req := &models.RefundPaymentRequest{Reference: "ref-1", Amount: 5000}
	payment := &store.Payment{PaymentState: "captured", AmountCents: 5000}

	stateErr := refundStateCheck(req, payment)

	require.NotNil(t, stateErr)
	require.Equal(t, http.StatusConflict, stateErr.StatusCode)
	require.Equal(t, "capture_not_recorded", stateErr.Code)
}

func TestRefundStateCheckRejectsAmountMismatch(t *testing.T) {
	req := &models.RefundPaymentRequest{Reference: "ref-1", Amount: 1000}
	payment := &store.Payment{
		PaymentState: "captured",
		AmountCents:  5000,
		CaptureID:    pgtype.Text{String: "cap_x", Valid: true},
	}

	stateErr := refundStateCheck(req, payment)

	require.NotNil(t, stateErr)
	require.Equal(t, http.StatusBadRequest, stateErr.StatusCode)
	require.Equal(t, "amount_mismatch", stateErr.Code)
}

func TestValidateRefundRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &models.RefundPaymentRequest{
			Reference: "550e8400-e29b-41d4-a716-446655440000",
			Amount:    5000,
		}
		require.NoError(t, ValidateRefundRequest(req))
	})

	t.Run("zero amount", func(t *testing.T) {
		req := &models.RefundPaymentRequest{
			Reference: "550e8400-e29b-41d4-a716-446655440000",
			Amount:    0,
		}
		require.Error(t, ValidateRefundRequest(req))
	})

	t.Run("non-uuid reference", func(t *testing.T) {
		req := &models.RefundPaymentRequest{Reference: "not-a-uuid", Amount: 100}
		require.Error(t, ValidateRefundRequest(req))
	})
}
