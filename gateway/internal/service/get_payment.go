package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
)

// GetPayment returns the stored payment record by its reference. Purely a
// read of the gateway's own records — no bank call, no idempotency ledger —
// so a payment reachable here is always the gateway's single source of truth.
func (s *PaymentService) GetPayment(ctx context.Context, reference string) (*models.PaymentResponse, error) {
	ref, err := uuid.Parse(reference)
	if err != nil {
		return nil, NewNotFoundError("payment reference must be a valid UUID")
	}

	row, err := s.repo.GetPaymentByReference(ctx, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, NewNotFoundError("No payment exists with the given reference")
		}
		return nil, NewInternalError(err)
	}

	res := buildPaymentResponse(&row)
	return &res, nil
}
