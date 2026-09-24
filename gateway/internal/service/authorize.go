package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/bankclient"
	"github.com/nehemiah-dev/payment-gateway/internal/database"
	"github.com/nehemiah-dev/payment-gateway/internal/domain"
	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

// Authorize processes a payment authorization request.
// The payment record is created before the bank call so a crash mid-flight
// leaves a PENDING row that can be reconciled, rather than a lost transaction.
func (s *PaymentService) Authorize(ctx context.Context, req *models.AuthorizePaymentRequest, idempotencyKey string) (*models.PaymentResponse, error) {
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

	var paymentResponse *models.PaymentResponse

	txErr := s.db.WithTx(ctx, func(txCtx context.Context, _ database.Querier) error {
		// Insert the PENDING record before touching the bank so the row exists
		// even if the process crashes after the bank responds.
		payment, err := s.repo.CreatePayment(txCtx, &store.CreatePaymentParams{
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
			zap.String("order_id", req.OrderID),
			zap.Int64("amount", req.Amount),
		)

		bankReq := bankclient.AuthorizeRequest{
			IdempotencyKey: idempotencyKey,
			CardNumber:     req.CardNumber,
			CVV:            req.CardCVV,
			ExpiryMonth:    expiryMonth,
			ExpiryYear:     expiryYear,
			AmountCents:    req.Amount,
		}

		bankResult, bankErr := s.bankClient.Authorize(txCtx, bankReq)

		if bankErr != nil {
			zap.L().Error(
				"bank authorization failed",
				zap.Error(bankErr),
				zap.String("payment_ref", payment.Reference.String()),
			)

			if transErr := domain.ValidateTransition(domain.StatePending, domain.StateFailed); transErr != nil {
				return fmt.Errorf("invalid state transition: %w", transErr)
			}

			_, err = s.repo.FailPayment(txCtx, payment.ID)
			if err != nil {
				return fmt.Errorf("mark payment as failed: %w", err)
			}

			return MapBankError(bankErr)
		}

		zap.L().Info(
			"bank authorization succeeded",
			zap.String("payment_ref", payment.Reference.String()),
			zap.String("bank_auth_id", bankResult.AuthorizationID),
		)

		if transErr := domain.ValidateTransition(domain.StatePending, domain.StateAuthorized); transErr != nil {
			return fmt.Errorf("invalid state transition: %w", transErr)
		}

		authorizedPayment, err := s.repo.AuthorizePayment(txCtx, store.AuthorizePaymentParams{
			ID:              payment.ID,
			AuthorizationID: pgtype.Text{String: bankResult.AuthorizationID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("update payment to authorized: %w", err)
		}

		paymentResponse = buildPaymentResponse(&authorizedPayment)
		return nil
	})

	if txErr != nil {
		return nil, txErr
	}

	return paymentResponse, nil
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

// buildPaymentResponse constructs the API response from database payment record.
func buildPaymentResponse(payment *store.Payment) *models.PaymentResponse {
	response := &models.PaymentResponse{
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

	if payment.PaymentState == string(domain.StateAuthorized) {
		response.AuthorizedAt = &payment.UpdatedAt
	}

	return response
}
