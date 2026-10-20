package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/bankclient"
	"github.com/nehemiah-dev/payment-gateway/internal/database"
	"github.com/nehemiah-dev/payment-gateway/internal/domain"
	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/repository"
	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

// Authorize processes a payment authorization request and owns the idempotency
// ledger write. Every statement the request produces runs on ONE transaction —
// the payment row AND the idempotency record — so exactly one of these
// outcomes commits:
//
//	success  → payment = authorized, key = complete  → replayable 200
//	declined → payment = failed,     key = complete  → replayable mapped 4xx
//	unknown  → everything rolls back (the payment row and the reserved key
//	           disappear) → the client may safely retry with the same key
//
// Because the ledger commits with the payment, any error above 2xx is
// unambiguous: either a definitive committed result that replays on retry, or
// a clean rollback that invites a retry.
//
// The payment record is created before the bank call so a crash mid-flight
// leaves a PENDING row that can be reconciled, rather than a lost transaction.
func (s *PaymentService) Authorize(ctx context.Context, req *models.AuthorizePaymentRequest) (*IdempotentResult, error) {
	if err := ValidateAuthorizationRequest(req); err != nil {
		return nil, NewValidationError(err.Error())
	}

	cardLastFour := extractLastFourDigits(req.CardNumber)

	expiryMonth, expiryYear, err := parseCardExpiry(req.CardExpiry)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("invalid card expiry format: %v", err))
	}

	metadata := map[string]interface{}{
		"card_expiry": req.CardExpiry,
	}
	metadataJSON, _ := json.Marshal(metadata)

	var result *IdempotentResult

	txErr := s.db.WithTx(ctx, func(txCtx context.Context, txQ database.Querier) error {
		// Repo bound to THIS transaction (not the pool) — a rollback must
		// really discard the payment row and the idempotency reservation.
		repo := repository.NewTxRepository(txQ)

		// 1) Reserve the (key, path) slot on this transaction.
		scope := scopeFromAuthorize(req)
		replay, err := claimIdempotency(txCtx, repo, scope)
		if err != nil {
			return err
		}
		if replay != nil {
			// The slot is claimed by a committed record — nothing to commit.
			result = replay
			return nil
		}

		// 2) Insert the PENDING record before touching the bank so the row
		// exists even if the process crashes after the bank responds.
		payment, err := repo.CreatePayment(txCtx, &store.CreatePaymentParams{
			AmountCents:  req.Amount,
			Currency:     req.Currency,
			CardLastFour: cardLastFour,
			CustomerID:   req.CustomerID,
			OrderID:      req.OrderID,
			Metadata:     metadataJSON,
		})
		if err != nil {
			return fmt.Errorf("create pending payment: %w", err)
		}

		zap.L().Info(
			"created pending payment",
			zap.String("payment_ref", payment.Reference.String()),
			zap.String("idempotency_key", req.GatewayIdempotencyKey),
			zap.String("order_id", req.OrderID),
			zap.Int64("amount", req.Amount),
		)

		bankReq := bankclient.AuthorizeRequest{
			IdempotencyKey: req.BankIdempotencyKey,
			CardNumber:     req.CardNumber,
			CVV:            req.CardCVV,
			ExpiryMonth:    expiryMonth,
			ExpiryYear:     expiryYear,
			AmountCents:    req.Amount,
		}

		// 3) Talk to the bank while holding the transaction — and therefore
		// the idempotency slot. Concurrent duplicates block on their INSERT
		// instead of double-charging, and resolve to replay or 409 once this
		// transaction settles.
		bankResult, bankErr := s.bankClient.Authorize(txCtx, bankReq)

		if bankErr != nil {
			mapped := MapBankError(bankErr)

			if outcomeUnknown(bankErr) {
				// No response, or the bank signaled retryable: the outcome
				// is unknowable from this side. Roll back the payment row and
				// the slot so the client can retry with the same key, relying
				// on the bank to dedupe by its idempotency key.
				zap.L().Error(
					"bank authorization failed, rolling back (outcome unknown)",
					zap.Error(bankErr),
					zap.String("payment_ref", payment.Reference.String()),
				)
				return mapped
			}

			// 4a) Definitive decline. Record FAILED and cache the mapped
			// error response atomically, so every retry replays the same
			// verdict instead of leaving the client guessing.
			decline, declineErr := mapped.(*PaymentError)
			if !declineErr {
				decline = NewInternalError(bankErr)
			}
			result, err = recordDecline(txCtx, repo, scope, &payment, decline, bankErr)
			return err
		}

		zap.L().Info(
			"bank authorization succeeded",
			zap.String("payment_ref", payment.Reference.String()),
			zap.String("idempotency_key", req.GatewayIdempotencyKey),
			zap.String("bank_auth_id", bankResult.AuthorizationID),
		)

		if transErr := domain.ValidateTransition(domain.StatePending, domain.StateAuthorized); transErr != nil {
			return fmt.Errorf("invalid state transition: %w", transErr)
		}

		authorizedPayment, err := repo.AuthorizePayment(txCtx, store.AuthorizePaymentParams{
			ID:              payment.ID,
			AuthorizationID: pgtype.Text{String: bankResult.AuthorizationID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("update payment to authorized: %w", err)
		}

		// 4b) Finalize the idempotency record with the SUCCESS response — on
		// the same transaction as the authorized payment row.
		result, err = finalizeIdempotentOutcome(txCtx, repo, scope, http.StatusOK, buildPaymentResponse(&authorizedPayment))
		return err
	})

	if txErr != nil {
		return nil, txErr
	}

	return result, nil
}

// outcomeUnknown reports whether the bank call produced no authoritative
// answer: the transport failed (no response at all) or the bank signaled a
// retryable error. In both cases nothing must be recorded — the client retries
// the same Idempotency-Key and the bank dedupes on its side.
func outcomeUnknown(bankErr error) bool {
	var bankResp *bankclient.Error
	return !errors.As(bankErr, &bankResp) || bankResp.Retryable
}

// recordDecline commits the FAILED payment state and finalizes the idempotency
// record with the mapped error response. A nil return lets the surrounding
// transaction commit, making the decline a replayable, definitive outcome.
func recordDecline(ctx context.Context, repo *repository.Repository, scope idempotencyScope, payment *store.Payment, decline *PaymentError, bankErr error) (*IdempotentResult, error) {
	zap.L().Error(
		"bank operation declined",
		zap.Error(bankErr),
		zap.String("payment_ref", payment.Reference.String()),
		zap.String("code", decline.Code),
	)

	if transErr := domain.ValidateTransition(domain.PaymentState(payment.PaymentState), domain.StateFailed); transErr != nil {
		return nil, fmt.Errorf("invalid state transition: %w", transErr)
	}

	if _, err := repo.FailPayment(ctx, payment.ID); err != nil {
		return nil, fmt.Errorf("mark payment as failed: %w", err)
	}

	return finalizeIdempotentOutcome(ctx, repo, scope, decline.StatusCode, models.ErrorResponse{
		Error:   decline.Code,
		Message: decline.Message,
		Code:    decline.Code,
	})
}

// finalizeIdempotentOutcome marshals the response body, finalizes the
// idempotency record owned by the current transaction, and returns the exact
// response the handler must write — identical to what a future replay gets.
func finalizeIdempotentOutcome(ctx context.Context, repo *repository.Repository, scope idempotencyScope, statusCode int, responseBody any) (*IdempotentResult, error) {
	body, err := json.Marshal(responseBody)
	if err != nil {
		return nil, NewInternalError(fmt.Errorf("marshal idempotency response body: %w", err))
	}

	if err := repo.FinalizeIdempotencyKey(ctx, scope.key, scope.path, statusCode, body); err != nil {
		return nil, NewInternalError(fmt.Errorf("finalize idempotency record: %w", err))
	}

	zap.L().Info(
		"finalized idempotency record",
		zap.String("idempotency_key", scope.key),
		zap.String("path", scope.path),
		zap.Int("status_code", statusCode),
	)

	return &IdempotentResult{StatusCode: statusCode, Body: body}, nil
}

func extractLastFourDigits(cardNumber string) string {
	cleaned := strings.ReplaceAll(cardNumber, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	if len(cleaned) >= 4 {
		return cleaned[len(cleaned)-4:]
	}
	return cleaned
}

// parseCardExpiry parses card expiry in format MM/YYYY.
func parseCardExpiry(expiry string) (month, year int, err error) {
	parts := strings.Split(expiry, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected format MM/YYYY")
	}

	month, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("invalid month")
	}

	year, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || year < 2000 || year > 2099 {
		return 0, 0, fmt.Errorf("invalid year")
	}

	return month, year, nil
}

// buildPaymentResponse constructs the API response from the database payment
// record. Its JSON shape matches the generated api.PaymentResponse so the
// serialized body can be cached in the idempotency ledger verbatim.
func buildPaymentResponse(payment *store.Payment) models.PaymentResponse {
	response := models.PaymentResponse{
		PaymentID:  fmt.Sprintf("%d", payment.ID),
		Reference:  payment.Reference.String(),
		OrderID:    payment.OrderID,
		CustomerID: payment.CustomerID,
		Amount:     payment.AmountCents,
		Currency:   payment.Currency,
		State:      payment.PaymentState,
		CreatedAt:  payment.CreatedAt,
	}

	if payment.AuthorizationID.Valid && payment.AuthorizationID.String != "" {
		authID := payment.AuthorizationID.String
		response.BankAuthID = &authID
	}

	if payment.CaptureID.Valid && payment.CaptureID.String != "" {
		captureID := payment.CaptureID.String
		response.BankCaptureID = &captureID
	}

	if payment.RefundID.Valid && payment.RefundID.String != "" {
		refundID := payment.RefundID.String
		response.BankRefundID = &refundID
	}

	if payment.PaymentState == string(domain.StateAuthorized) {
		response.AuthorizedAt = &payment.UpdatedAt
	}

	if payment.PaymentState == string(domain.StateCaptured) {
		response.CapturedAt = &payment.UpdatedAt
	}

	if payment.PaymentState == string(domain.StateRefunded) {
		response.RefundedAt = &payment.UpdatedAt
	}

	return response
}
