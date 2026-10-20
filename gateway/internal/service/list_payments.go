package service

import (
	"context"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

const (
	defaultListPaymentLimit = 50
	maxListPaymentLimit     = 100
)

// ListPayments returns stored payments filtered by exactly one of customer or
// order, newest first. Purely a read of the gateway's own records — no bank
// call, no idempotency ledger.
func (s *PaymentService) ListPayments(ctx context.Context, req *models.ListPaymentsRequest) ([]models.PaymentResponse, error) {
	customerID, orderID, limit, offset, err := resolveListFilter(req)
	if err != nil {
		return nil, err
	}

	var rows []store.Payment
	if customerID != "" {
		rows, err = s.repo.ListPaymentsByCustomer(ctx, store.ListPaymentsByCustomerParams{
			CustomerID: customerID,
			Limit:      asStoreLimit(limit),
			Offset:     asStoreOffset(offset),
		})
	} else {
		rows, err = s.repo.ListPaymentsByOrder(ctx, store.ListPaymentsByOrderParams{
			OrderID: orderID,
			Limit:   asStoreLimit(limit),
			Offset:  asStoreOffset(offset),
		})
	}
	if err != nil {
		return nil, NewInternalError(err)
	}

	results := make([]models.PaymentResponse, len(rows))
	for i := range rows {
		results[i] = buildPaymentResponse(&rows[i])
	}
	return results, nil
}

// resolveListFilter validates that exactly one filter is set and normalizes
// paging (bounded limit, non-negative offset). Pure so it can be tested
// without a DB.
func resolveListFilter(req *models.ListPaymentsRequest) (customerID, orderID string, limit, offset int, err error) {
	if req.CustomerID != "" && req.OrderID != "" {
		return "", "", 0, 0, NewValidationError("provide either customer_id or order_id, not both")
	}
	if req.CustomerID == "" && req.OrderID == "" {
		return "", "", 0, 0, NewValidationError("one of customer_id or order_id is required")
	}

	limit, offset = req.Limit, req.Offset
	if limit <= 0 {
		limit = defaultListPaymentLimit
	}
	if limit > maxListPaymentLimit {
		limit = maxListPaymentLimit
	}
	if offset < 0 {
		offset = 0
	}

	return req.CustomerID, req.OrderID, limit, offset, nil
}

// asStoreLimit converts a normalized paging limit to the store's int32
// paging value. resolveListFilter guarantees the value is within
// [1, maxListPaymentLimit], so the conversion cannot overflow.
func asStoreLimit(limit int) int32 { return int32(limit) } //nolint:gosec // clamped by resolveListFilter

// asStoreOffset converts a normalized paging offset to the store's int32
// paging value. resolveListFilter guarantees the value is non-negative, so
// the conversion cannot overflow.
func asStoreOffset(offset int) int32 { return int32(offset) } //nolint:gosec // clamped by resolveListFilter
