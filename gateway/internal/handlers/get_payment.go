package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// GetPayment handles GET /api/v1/payments/{reference}
// Implements the generated ServerInterface from OpenAPI
func (s *Server) GetPayment(c *gin.Context, reference openapi_types.UUID) {
	res, err := s.paymentService.GetPayment(c.Request.Context(), reference.String())
	if err != nil {
		handleServiceError(c, err)
		return
	}

	// models.PaymentResponse carries the same JSON shape as the generated
	// api.PaymentResponse, so serializing it directly produces the same
	// bytes the write endpoints cache in the idempotency ledger.
	c.JSON(http.StatusOK, res)
}
