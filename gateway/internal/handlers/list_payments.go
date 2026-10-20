package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/models"
)

// ListPayments handles GET /api/v1/payments
// Implements the generated ServerInterface from OpenAPI
func (s *Server) ListPayments(c *gin.Context, params api.ListPaymentsParams) {
	res, err := s.paymentService.ListPayments(c.Request.Context(), &models.ListPaymentsRequest{
		CustomerID: params.CustomerId,
		OrderID:    params.OrderId,
		Limit:      params.Limit,
		Offset:     params.Offset,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}

	// models.PaymentResponse carries the same JSON shape as the generated
	// api.PaymentResponse, so serializing it directly produces the same
	// bytes the write endpoints cache in the idempotency ledger.
	c.JSON(http.StatusOK, res)
}
