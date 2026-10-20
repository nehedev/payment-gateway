package handlers

import (
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/middleware"
	"github.com/nehemiah-dev/payment-gateway/internal/models"
)

// VoidPayment handles POST /api/v1/payments/{reference}/void
// Implements the generated ServerInterface from OpenAPI
func (s *Server) VoidPayment(c *gin.Context, reference openapi_types.UUID, params api.VoidPaymentParams) {
	// Convert API request to internal service model. The void carries no
	// request body: the full authorized amount is canceled and the
	// authorization ID is taken from the stored payment row.
	req := &models.VoidPaymentRequest{
		// The reference identifies the payment to void; taken from the
		// request path, never from a body.
		Reference: reference.String(),
		// Forward the idempotency key to the bank so it can deduplicate the
		// void on its side.
		BankIdempotencyKey: params.IdempotencyKey,
		// The gateway's own idempotency coordinates, stamped by the
		// middleware. The service persists the ledger row for (key, path)
		// inside the same transaction that commits the void state.
		GatewayIdempotencyKey:      c.GetString(middleware.CtxIdempotencyKey),
		GatewayIdempotencyPath:     c.Request.URL.Path,
		GatewayIdempotencyBodyHash: c.GetString(middleware.CtxIdempotencyBodyHash),
	}

	// Call service layer
	ctx := c.Request.Context()
	res, err := s.paymentService.Void(ctx, req)
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
