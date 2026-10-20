package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
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
		// side. Replay decisions at the gateway level are owned by the middleware.
		BankIdempotencyKey: params.IdempotencyKey,
	}

	// Call service layer
	ctx := c.Request.Context()
	payment, err := s.paymentService.Authorize(ctx, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	// Convert internal model to API response
	refUUID, _ := uuid.Parse(payment.Reference)
	apiResponse := api.PaymentResponse{
		PaymentId:  payment.PaymentID,
		Reference:  refUUID,
		OrderId:    payment.OrderID,
		CustomerId: payment.CustomerID,
		Amount:     payment.Amount,
		Currency:   payment.Currency,
		State:      api.PaymentState(payment.State),
		CreatedAt:  payment.CreatedAt,
	}

	if payment.BankAuthID != nil {
		apiResponse.BankAuthId = *payment.BankAuthID
	}

	if payment.AuthorizedAt != nil {
		apiResponse.AuthorizedAt = *payment.AuthorizedAt
	}

	c.JSON(http.StatusOK, apiResponse)
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
