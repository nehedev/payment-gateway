package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
)

func TestResolveListFilterRequiresExactlyOneFilter(t *testing.T) {
	t.Run("both filters rejected", func(t *testing.T) {
		req := &models.ListPaymentsRequest{CustomerID: "c1", OrderID: "o1"}
		_, _, _, _, err := resolveListFilter(req)
		require.Error(t, err)
		var pe *PaymentError
		require.ErrorAs(t, err, &pe)
		require.Equal(t, 400, pe.StatusCode)
	})

	t.Run("no filter rejected", func(t *testing.T) {
		req := &models.ListPaymentsRequest{}
		_, _, _, _, err := resolveListFilter(req)
		require.Error(t, err)
	})
}

func TestResolveListFilterNormalizesPaging(t *testing.T) {
	t.Run("defaults applied", func(t *testing.T) {
		req := &models.ListPaymentsRequest{CustomerID: "c1"}
		customerID, orderID, limit, offset, err := resolveListFilter(req)
		require.NoError(t, err)
		require.Equal(t, "c1", customerID)
		require.Empty(t, orderID)
		require.Equal(t, int(50), limit)
		require.Equal(t, int(0), offset)
	})

	t.Run("limit clamped to max", func(t *testing.T) {
		req := &models.ListPaymentsRequest{OrderID: "o1", Limit: 500, Offset: 10}
		_, _, limit, offset, err := resolveListFilter(req)
		require.NoError(t, err)
		require.Equal(t, int(100), limit)
		require.Equal(t, int(10), offset)
	})

	t.Run("negative offset clamped", func(t *testing.T) {
		req := &models.ListPaymentsRequest{OrderID: "o1", Offset: -5}
		_, _, _, offset, err := resolveListFilter(req)
		require.NoError(t, err)
		require.Equal(t, int(0), offset)
	})
}
