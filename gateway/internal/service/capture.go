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

// Capture captures a previously authorized payment and owns the idempotency
// ledger write, following the same transactional pattern as Authorize: every
// statement runs on ONE transaction — the payment row AND the idempotency
// record — so exactly one of these outcomes commits:
//
//	success   → payment = captured,     key = complete → replayable 200
//	declined  → payment = failed (when the authorization is no longer
//	            usable) or untouched (client-fixable defects), key = complete
//	            → replayable mapped 4xx
//	unknown   → everything rolls back → retry allowed with the same key
//
// The authorization ID sent to the bank is taken from the stored payment row,
// never from the client, so a capture can only ever reference what the gateway
// actually authorized.
func (s *PaymentService) Capture(ctx context.Context, req *models.CapturePaymentRequest) (*IdempotentResult, error) {
	if err := ValidateCaptureRequest(req); err != nil {
		return nil, NewValidationError(err.Error())
	}

	var result *IdempotentResult

	txErr := s.db.WithTx(ctx, func(txCtx context.Context, txQ database.Querier) error {
		// Repo bound to THIS transaction (not the pool) — a rollback must
		// really discard the capture state and the idempotency reservation.
		repo := repository.NewTxRepository(txQ)
		scope := scopeFromCapture(req)

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
					"payment not found for capture",
					zap.String("reference", req.Reference),
				)
				result, err = recordLocalDecline(txCtx, repo, scope, http.StatusNotFound,
					"payment_not_found", "No payment exists with the given reference")
				return err
			}
			return fmt.Errorf("load payment: %w", err)
		}

		if stateErr := captureStateCheck(req, &payment); stateErr != nil {
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

		bankReq := bankclient.CaptureRequest{
			IdempotencyKey:  req.BankIdempotencyKey,
			AuthorizationID: payment.AuthorizationID.String,
			AmountCents:     req.Amount,
		}

		// 3) Talk to the bank while holding the transaction — and therefore
		// the idempotency slot. Concurrent duplicates block on their INSERT
		// instead of double-capturing.
		bankCapture, bankErr := s.bankClient.Capture(txCtx, bankReq)

		if bankErr != nil {
			mapped := MapBankError(bankErr)

			if outcomeUnknown(bankErr) {
				// No response, or the bank signaled retryable: the outcome
				// is unknowable from this side. Roll back the capture state
				// and the slot so the client can retry with the same key,
				// relying on the bank to dedupe by its idempotency key.
				zap.L().Error(
					"bank capture failed, rolling back (outcome unknown)",
					zap.Error(bankErr),
					zap.String("payment_ref", payment.Reference.String()),
				)
				return mapped
			}

			// 4a) Definitive decline.
			decline, declineErr := mapped.(*PaymentError)
			if !declineErr {
				decline = NewInternalError(bankErr)
			}

			if authUnusable(bankErr) {
				// The authorization can no longer be captured — record FAILED
				// and cache the mapped error response atomically, so every
				// retry replays the same verdict.
				result, err = recordDecline(txCtx, repo, scope, &payment, decline, bankErr)
				return err
			}

			// Client-fixable defect (e.g. amount mismatch): the authorization
			// stays intact for a retry with a fresh key; only the outcome is
			// cached.
			result, err = recordLocalDecline(txCtx, repo, scope, decline.StatusCode, decline.Code, decline.Message)
			return err
		}

		zap.L().Info(
			"bank capture succeeded",
			zap.String("payment_ref", payment.Reference.String()),
			zap.String("idempotency_key", req.GatewayIdempotencyKey),
			zap.String("bank_capture_id", bankCapture.CaptureID),
		)

		// 4b) Mark captured and finalize the idempotency record with the
		// SUCCESS response — on the same transaction as the captured row.
		capturedPayment, err := repo.CapturePayment(txCtx, store.CapturePaymentParams{
			ID:        payment.ID,
			CaptureID: pgtype.Text{String: bankCapture.CaptureID, Valid: true},
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// The state changed between our SELECT and this guarded
				// UPDATE — a concurrent capture with a different key. ROLL
				// BACK: the retry replays the local state verdict instead.
				zap.L().Warn(
					"payment no longer in authorized state for capture",
					zap.Error(err),
					zap.String("payment_ref", payment.Reference.String()),
				)
				return NewBankDeclineError(bankclient.ErrorAlreadyCaptured,
					"payment has already been captured or is no longer authorized", http.StatusConflict)
			}
			return fmt.Errorf("update payment to captured: %w", err)
		}

		result, err = finalizeIdempotentOutcome(txCtx, repo, scope, http.StatusOK, buildPaymentResponse(&capturedPayment))
		return err
	})

	if txErr != nil {
		return nil, txErr
	}

	return result, nil
}

// captureStateCheck verifies the stored payment row is locally captureable
// before spending a bank call: only payments in the authorized state may move
// to captured, and the capture amount must match the authorized amount.
func captureStateCheck(req *models.CapturePaymentRequest, payment *store.Payment) *PaymentError {
	if payment.PaymentState != string(domain.StateAuthorized) {
		return &PaymentError{
			StatusCode: http.StatusConflict,
			Code:       "payment_not_authorized",
			Message: fmt.Sprintf(
				"payment is in %s state; only authorized payments can be captured",
				payment.PaymentState,
			),
		}
	}
	if req.Amount != payment.AmountCents {
		return &PaymentError{
			StatusCode: http.StatusBadRequest,
			Code:       "amount_mismatch",
			Message: fmt.Sprintf(
				"capture amount %d does not match authorized amount %d",
				req.Amount, payment.AmountCents,
			),
		}
	}
	return nil
}

// authUnusable reports whether a definitive bank decline means the underlying
// authorization can no longer be used — the payment must move to failed from
// its currently stored hold state (pending/authorized; money has not moved) —
// as opposed to a client-fixable defect (amount mismatch) that leaves the
// authorization intact for a retry with a fresh key.
func authUnusable(bankErr error) bool {
	var bankResp *bankclient.Error
	if !errors.As(bankErr, &bankResp) {
		return false
	}

	switch bankResp.Code {
	case bankclient.ErrorAuthorizationNotFound,
		bankclient.ErrorAuthorizationExpired,
		bankclient.ErrorAuthorizationAlreadyUsed,
		bankclient.ErrorAlreadyCaptured,
		bankclient.ErrorAlreadyVoided:
		return true
	default:
		return false
	}
}

// recordLocalDecline caches a deterministic, non-state-changing rejection
// (payment not found, non-agreeing amount, ...) so retries replay it, and
// returns a nil error so the surrounding transaction commits.
func recordLocalDecline(ctx context.Context, repo *repository.Repository, scope idempotencyScope, statusCode int, code, message string) (*IdempotentResult, error) {
	return finalizeIdempotentOutcome(ctx, repo, scope, statusCode, models.ErrorResponse{
		Error:   code,
		Message: message,
		Code:    code,
	})
}
