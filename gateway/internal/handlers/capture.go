package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/middleware"
	"github.com/nehemiah-dev/payment-gateway/internal/models"
)

// CapturePayment handles POST /api/v1/payments/{reference}/capture
// Implements the generated ServerInterface from OpenAPI
func (s *Server) CapturePayment(c *gin.Context, reference openapi_types.UUID, params api.CapturePaymentParams) {
	// Bind request body to generated API model
	var apiReq api.CapturePaymentRequest
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

	// Convert API request to internal service model.
	req := &models.CapturePaymentRequest{
		// The reference identifies the payment to capture; taken from the
		// request path, never from the body.
		Reference: reference.String(),
		Amount:    apiReq.Amount,
		// Forward the idempotency key to the bank so it can deduplicate the
		// capture on its side.
		BankIdempotencyKey: params.IdempotencyKey,
		// The gateway's own idempotency coordinates, stamped by the
		// middleware. The service persists the ledger row for (key, path)
		// inside the same transaction that commits the capture state.
		GatewayIdempotencyKey:      c.GetString(middleware.CtxIdempotencyKey),
		GatewayIdempotencyPath:     c.Request.URL.Path,
		GatewayIdempotencyBodyHash: c.GetString(middleware.CtxIdempotencyBodyHash),
	}

	// Call service layer
	ctx := c.Request.Context()
	res, err := s.paymentService.Capture(ctx, req)
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
