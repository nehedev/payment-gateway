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

// Void cancels a previously authorized payment before capture and owns the
// idempotency ledger write, following the same transactional pattern as
// Authorize, Capture and Refund: every statement runs on ONE transaction —
// the payment row AND the idempotency record — so exactly one of these
// outcomes commits:
//
//	success  → payment = voided,   key = complete → replayable 200
//	declined → payment = failed when the stored hold is no longer usable at
//	           the bank (money has NOT moved on a void decline, so failed is
//	           the honest terminal state), key = complete → replayable 4xx
//	unknown  → everything rolls back → retry allowed with the same key
//
// The authorization ID sent to the bank is taken from the stored payment row,
// never from the client, and the void is full-amount by definition.
func (s *PaymentService) Void(ctx context.Context, req *models.VoidPaymentRequest) (*IdempotentResult, error) {
	if err := ValidateVoidRequest(req); err != nil {
		return nil, NewValidationError(err.Error())
	}

	var result *IdempotentResult

	txErr := s.db.WithTx(ctx, func(txCtx context.Context, txQ database.Querier) error {
		// Repo bound to THIS transaction (not the pool) — a rollback must
		// really discard the void state and the idempotency reservation.
		repo := repository.NewTxRepository(txQ)
		scope := scopeFromVoid(req)

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
					"payment not found for void",
					zap.String("reference", req.Reference),
				)
				result, err = recordLocalDecline(txCtx, repo, scope, http.StatusNotFound,
					"payment_not_found", "No payment exists with the given reference")
				return err
			}
			return fmt.Errorf("load payment: %w", err)
		}

		if stateErr := voidStateCheck(&payment); stateErr != nil {
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

		bankReq := bankclient.VoidRequest{
			IdempotencyKey:  req.BankIdempotencyKey,
			AuthorizationID: payment.AuthorizationID.String,
		}

		// 3) Talk to the bank while holding the transaction — and therefore
		// the idempotency slot. Concurrent duplicates block on their INSERT
		// instead of double-voiding.
		bankVoid, bankErr := s.bankClient.Void(txCtx, bankReq)

		if bankErr != nil {
			mapped := MapBankError(bankErr)

			if outcomeUnknown(bankErr) {
				// No response, or the bank signaled retryable: the outcome
				// is unknowable from this side. Roll back the void state
				// and the slot so the client can retry with the same key,
				// relying on the bank to dedupe by its idempotency key.
				zap.L().Error(
					"bank void failed, rolling back (outcome unknown)",
					zap.Error(bankErr),
					zap.String("payment_ref", payment.Reference.String()),
				)
				return mapped
			}

			// 4a) Definitive decline. VOID moves no money, so a dead hold
			// (authorization not found/expired/used, already captured or
			// voided) lets the payment move authorized → failed — the same
			// rule a failed capture follows. The outcome is cached
			// atomically, so every retry replays the same verdict.
			decline, declineErr := mapped.(*PaymentError)
			if !declineErr {
				decline = NewInternalError(bankErr)
			}
			result, err = recordDecline(txCtx, repo, scope, &payment, decline, bankErr)
			return err
		}

		zap.L().Info(
			"bank void succeeded",
			zap.String("payment_ref", payment.Reference.String()),
			zap.String("idempotency_key", req.GatewayIdempotencyKey),
			zap.String("bank_void_id", bankVoid.VoidID),
		)

		// 4b) Mark voided and finalize the idempotency record with the
		// SUCCESS response — on the same transaction as the voided row.
		voidedPayment, err := repo.VoidPayment(txCtx, store.VoidPaymentParams{
			ID:     payment.ID,
			VoidID: pgtype.Text{String: bankVoid.VoidID, Valid: true},
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// The state changed between our SELECT and this guarded
				// UPDATE — a concurrent void with a different key. ROLL
				// BACK: the retry replays the local state verdict instead.
				zap.L().Warn(
					"payment no longer in authorized state for void",
					zap.Error(err),
					zap.String("payment_ref", payment.Reference.String()),
				)
				return NewBankDeclineError(bankclient.ErrorAuthorizationAlreadyUsed,
					"payment has already been voided or is no longer authorized", http.StatusConflict)
			}
			return fmt.Errorf("update payment to voided: %w", err)
		}

		result, err = finalizeIdempotentOutcome(txCtx, repo, scope, http.StatusOK, buildPaymentResponse(&voidedPayment))
		return err
	})

	if txErr != nil {
		return nil, txErr
	}

	return result, nil
}

// voidStateCheck verifies the stored payment row is locally voidable before
// spending a bank call: only payments in the authorized state may move to
// voided, and the recorded authorization that is being canceled must exist.
func voidStateCheck(payment *store.Payment) *PaymentError {
	if payment.PaymentState != string(domain.StateAuthorized) {
		return &PaymentError{
			StatusCode: http.StatusConflict,
			Code:       "payment_not_authorized",
			Message: fmt.Sprintf(
				"payment is in %s state; only authorized payments can be voided",
				payment.PaymentState,
			),
		}
	}
	if !payment.AuthorizationID.Valid || payment.AuthorizationID.String == "" {
		return &PaymentError{
			StatusCode: http.StatusConflict,
			Code:       "authorization_not_recorded",
			Message:    "payment has no recorded authorization; it cannot be voided",
		}
	}
	return nil
}
