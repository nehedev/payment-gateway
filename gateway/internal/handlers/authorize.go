package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/middleware"
	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/service"
)

// AuthorizePayment handles POST /api/v1/payments/authorize
// Implements the generated ServerInterface from OpenAPI
func (s *Server) AuthorizePayment(c *gin.Context, params api.AuthorizePaymentParams) {
	// Bind request body to generated API model
	var apiReq api.AuthorizePaymentRequest
	if err := c.ShouldBindJSON(&apiReq); err != nil {
		zap.L().Warn(
			"invalid request body",
			zap.Error(err),
			zap.String("request_id", c.GetString("request_id")),
		)
		c.JSON(http.StatusBadRequest, api.ErrorResponse{
			Error:   "validation_error",
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Convert API request to internal service model
	req := &models.AuthorizePaymentRequest{
		OrderID:    apiReq.OrderId,
		CustomerID: apiReq.CustomerId,
		Amount:     apiReq.Amount,
		Currency:   apiReq.Currency,
		CardNumber: apiReq.CardNumber,
		CardExpiry: apiReq.CardExpiry,
		CardCVV:    apiReq.CardCvv,
		// Forward the idempotency key to the bank so it can deduplicate on its
		// side.
		BankIdempotencyKey: params.IdempotencyKey,
		// The gateway's own idempotency coordinates, stamped by the
		// middleware. The service persists the ledger row for (key, path)
		// inside the same transaction that commits the payment record.
		GatewayIdempotencyKey:      c.GetString(middleware.CtxIdempotencyKey),
		GatewayIdempotencyPath:     c.Request.URL.Path,
		GatewayIdempotencyBodyHash: c.GetString(middleware.CtxIdempotencyBodyHash),
	}

	// Call service layer
	ctx := c.Request.Context()
	res, err := s.paymentService.Authorize(ctx, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	// The service owns the cached response: write the exact bytes it stored
	// in (or replayed from) the idempotency ledger, so the original response
	// and any replay are identical.
	if res.Replayed {
		c.Header(middleware.ReplayedHeader, "true")
	}
	c.Data(res.StatusCode, "application/json", res.Body)
}

// handleServiceError maps service errors to HTTP responses
func handleServiceError(c *gin.Context, err error) {
	var serviceErr *service.PaymentError
	if errors.As(err, &serviceErr) {
		zap.L().Info(
			"service error",
			zap.String("request_id", c.GetString("request_id")),
			zap.String("code", serviceErr.Code),
			zap.String("message", serviceErr.Message),
			zap.Int("status_code", serviceErr.StatusCode),
		)

		c.JSON(serviceErr.StatusCode, api.ErrorResponse{
			Error:   serviceErr.Code,
			Message: serviceErr.Message,
			Code:    serviceErr.Code,
		})
		return
	}

	// Unknown error - log and return 500
	zap.L().Error(
		"unexpected error",
		zap.Error(err),
		zap.String("request_id", c.GetString("request_id")),
	)
	c.JSON(http.StatusInternalServerError, api.ErrorResponse{
		Error:   "internal_error",
		Message: "An unexpected error occurred",
	})
}
