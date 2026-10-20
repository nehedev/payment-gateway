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

func TestVoidStateCheckRejectsNonAuthorizedPaymentState(t *testing.T) {
	states := []string{"pending", "captured", "voided", "refunded", "failed"}
	for _, state := range states {
		payment := &store.Payment{
			PaymentState: state,
			AmountCents:  5000,
			AuthorizationID: pgtype.Text{
				String: "auth_550e8400", Valid: true,
			},
		}

		stateErr := voidStateCheck(payment)

		require.NotNil(t, stateErr, "state %s should be non-voidable", state)
		require.Equal(t, http.StatusConflict, stateErr.StatusCode)
		require.Equal(t, "payment_not_authorized", stateErr.Code)
	}
}

func TestVoidStateCheckAcceptsAuthorizedPayment(t *testing.T) {
	payment := &store.Payment{
		PaymentState:    "authorized",
		AmountCents:     5000,
		AuthorizationID: pgtype.Text{String: "auth_550e8400", Valid: true},
	}

	require.Nil(t, voidStateCheck(payment))
}

func TestVoidStateCheckRejectsMissingRecordedAuthorization(t *testing.T) {
	payment := &store.Payment{PaymentState: "authorized", AmountCents: 5000}

	stateErr := voidStateCheck(payment)

	require.NotNil(t, stateErr)
	require.Equal(t, http.StatusConflict, stateErr.StatusCode)
	require.Equal(t, "authorization_not_recorded", stateErr.Code)
}

func TestAuthUnusableAddsVoidDeclines(t *testing.T) {
	// Void moves no money: all of these mean the stored authorization hold is
	// dead and the payment must move authorized -> failed.
	unusable := []bankclient.ErrorCode{
		bankclient.ErrorAuthorizationNotFound,
		bankclient.ErrorAuthorizationExpired,
		bankclient.ErrorAuthorizationAlreadyUsed,
		bankclient.ErrorAlreadyCaptured,
		bankclient.ErrorAlreadyVoided,
	}
	for _, code := range unusable {
		require.True(t, authUnusable(&bankclient.Error{Code: code}),
			"expected %s to mark the authorization unusable", code)
	}
}

func TestValidateVoidRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := &models.VoidPaymentRequest{
			Reference: "550e8400-e29b-41d4-a716-446655440000",
		}
		require.NoError(t, ValidateVoidRequest(req))
	})

	t.Run("non-uuid reference", func(t *testing.T) {
		req := &models.VoidPaymentRequest{Reference: "not-a-uuid"}
		require.Error(t, ValidateVoidRequest(req))
	})
}
