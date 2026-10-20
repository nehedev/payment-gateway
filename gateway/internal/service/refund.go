package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/bankclient"
	"github.com/nehemiah-dev/payment-gateway/internal/database"
	"github.com/nehemiah-dev/payment-gateway/internal/domain"
	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/repository"
	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

// Refund refunds a previously captured payment and owns the idempotency
// ledger write, following the same transactional pattern as Authorize and
// Capture: every statement runs on ONE transaction — the payment row AND the
// idempotency record — so exactly one of these outcomes commits:
//
//	success  → payment = refunded, key = complete → replayable 200
//	declined → payment untouched (captured means the money has already
//	           moved; a failed refund leaves it captured — only refunding
//	           can move it further), key = complete → replayable mapped 4xx
//	unknown  → everything rolls back → retry allowed with the same key
//
// The capture ID sent to the bank is taken from the stored payment row, never
// from the client, so a refund can only ever reference what the gateway
// actually captured.
func (s *PaymentService) Refund(ctx context.Context, req *models.RefundPaymentRequest) (*IdempotentResult, error) {
	if err := ValidateRefundRequest(req); err != nil {
		return nil, NewValidationError(err.Error())
	}

	var result *IdempotentResult

	txErr := s.db.WithTx(ctx, func(txCtx context.Context, txQ database.Querier) error {
		// Repo bound to THIS transaction (not the pool) — a rollback must
		// really discard the refund state and the idempotency reservation.
		repo := repository.NewTxRepository(txQ)
		scope := scopeFromRefund(req)

		// 1) Reserve the (key, path) slot on this transaction.
		replay, err := claimIdempotency(txCtx, repo, scope)
		if err != nil {
			return err
		}
		if replay != nil {
			// The slot is claimed by a committed record — nothing to commit.
			result = replay
			return nil
		}

		// 2) Load the payment and validate locally before touching the bank.
		ref, refErr := uuid.Parse(req.Reference)
		if refErr != nil {
			return NewValidationError("payment reference must be a valid UUID")
		}
		payment, err := repo.GetPaymentByReference(txCtx, ref)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Deterministic: this reference will never resolve. Cache the
				// 404 so retries with the same key replay it.
				zap.L().Warn(
					"payment not found for refund",
					zap.String("reference", req.Reference),
				)
				result, err = recordLocalDecline(txCtx, repo, scope, http.StatusNotFound,
					"payment_not_found", "No payment exists with the given reference")
				return err
			}
			return fmt.Errorf("load payment: %w", err)
		}

		if stateErr := refundStateCheck(req, &payment); stateErr != nil {
			// Deterministic rejections — the payment row already tells us the
			// verdict, so no bank call is needed and no state changes. Cache
			// the outcome so retries replay it.
			result, err = finalizeIdempotentOutcome(txCtx, repo, scope, stateErr.StatusCode, models.ErrorResponse{
				Error:   stateErr.Code,
				Message: stateErr.Message,
				Code:    stateErr.Code,
			})
			return err
		}

		bankReq := bankclient.RefundRequest{
			IdempotencyKey: req.BankIdempotencyKey,
			CaptureID:      payment.CaptureID.String,
			AmountCents:    req.Amount,
		}

		// 3) Talk to the bank while holding the transaction — and therefore
		// the idempotency slot. Concurrent duplicates block on their INSERT
		// instead of double-refunding.
		bankRefund, bankErr := s.bankClient.Refund(txCtx, bankReq)

		if bankErr != nil {
			mapped := MapBankError(bankErr)

			if outcomeUnknown(bankErr) {
				// No response, or the bank signaled retryable: the outcome
				// is unknowable from this side. Roll back the refund state
				// and the slot so the client can retry with the same key,
				// relying on the bank to dedupe by its idempotency key.
				zap.L().Error(
					"bank refund failed, rolling back (outcome unknown)",
					zap.Error(bankErr),
					zap.String("payment_ref", payment.Reference.String()),
				)
				return mapped
			}

			// 4a) Definitive decline. The payment is never failed here:
			// capture means the money has already moved, so captured can
			// only ever be refunded. The decline outcome is cached
			// atomically so every retry replays the same verdict — drift
			// cases (the bank replying the capture is not found or already
			// refunded) surface through the recorded conflict and need
			// reconciliation without silently rewriting the payment state.
			decline, declineErr := mapped.(*PaymentError)
			if !declineErr {
				decline = NewInternalError(bankErr)
			}
			result, err = recordLocalDecline(txCtx, repo, scope, decline.StatusCode, decline.Code, decline.Message)
			return err
		}

		zap.L().Info(
			"bank refund succeeded",
			zap.String("payment_ref", payment.Reference.String()),
			zap.String("idempotency_key", req.GatewayIdempotencyKey),
			zap.String("bank_refund_id", bankRefund.RefundID),
		)

		// 4b) Mark refunded and finalize the idempotency record with the
		// SUCCESS response — on the same transaction as the refunded row.
		refundedPayment, err := repo.RefundPayment(txCtx, store.RefundPaymentParams{
			ID:       payment.ID,
			RefundID: pgtype.Text{String: bankRefund.RefundID, Valid: true},
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// The state changed between our SELECT and this guarded
				// UPDATE — a concurrent refund with a different key. ROLL
				// BACK: the retry replays the local state verdict instead.
				zap.L().Warn(
					"payment no longer in captured state for refund",
					zap.Error(err),
					zap.String("payment_ref", payment.Reference.String()),
				)
				return NewBankDeclineError(bankclient.ErrorAlreadyRefunded,
					"payment has already been refunded or is no longer captured", http.StatusConflict)
			}
			return fmt.Errorf("update payment to refunded: %w", err)
		}

		result, err = finalizeIdempotentOutcome(txCtx, repo, scope, http.StatusOK, buildPaymentResponse(&refundedPayment))
		return err
	})

	if txErr != nil {
		return nil, txErr
	}

	return result, nil
}

// refundStateCheck verifies the stored payment row is locally refundable
// before spending a bank call: only payments in the captured state may move
// to refunded, the recorded capture must exist at the bank, and the refund
// amount must match the captured amount.
func refundStateCheck(req *models.RefundPaymentRequest, payment *store.Payment) *PaymentError {
	if payment.PaymentState != string(domain.StateCaptured) {
		return &PaymentError{
			StatusCode: http.StatusConflict,
			Code:       "payment_not_captured",
			Message: fmt.Sprintf(
				"payment is in %s state; only captured payments can be refunded",
				payment.PaymentState,
			),
		}
	}
	if !payment.CaptureID.Valid || payment.CaptureID.String == "" {
		return &PaymentError{
			StatusCode: http.StatusConflict,
			Code:       "capture_not_recorded",
			Message:    "payment has no recorded capture; it cannot be refunded",
		}
	}
	if req.Amount != payment.AmountCents {
		return &PaymentError{
			StatusCode: http.StatusBadRequest,
			Code:       "amount_mismatch",
			Message: fmt.Sprintf(
				"refund amount %d does not match captured amount %d",
				req.Amount, payment.AmountCents,
			),
		}
	}
	return nil
}
