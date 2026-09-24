package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/database"
	"github.com/nehemiah-dev/payment-gateway/internal/service"
)

type Server struct {
	db             *database.DB
	paymentService *service.PaymentService
}

func NewServer(db *database.DB, paymentService *service.PaymentService) *Server {
	return &Server{
		db:             db,
		paymentService: paymentService,
	}
}

// GetHealth handles GET /health — LIVENESS only, no DB dependency.
func (s *Server) GetHealth(c *gin.Context) {
	status := api.Healthy
	c.JSON(http.StatusOK, api.HealthResponse{Status: status})
}

func (s *Server) GetReadiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unavailable",
			"error":  "database unreachable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
