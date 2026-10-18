package router

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/bankclient"
	"github.com/nehemiah-dev/payment-gateway/internal/config"
	"github.com/nehemiah-dev/payment-gateway/internal/database"
	"github.com/nehemiah-dev/payment-gateway/internal/handlers"
	"github.com/nehemiah-dev/payment-gateway/internal/middleware"
	"github.com/nehemiah-dev/payment-gateway/internal/repository"
	"github.com/nehemiah-dev/payment-gateway/internal/service"
)

type Config struct {
	Environment    string
	AllowedOrigins []string
}

func New(db *database.DB, appConfig *config.Config, cfg Config) *gin.Engine {
	r := gin.New()

	r.Use(middleware.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID", "Idempotency-Key"},
		ExposeHeaders:    []string{"X-Request-ID", "X-Idempotent-Replayed"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	r.Use(middleware.RequestID())
	r.Use(middleware.Logger())

	// Initialize repositories
	paymentRepo := repository.NewRepository(db.Pool())
	idempotencyRepo := repository.NewIdempotencyRepository(db)

	// Initialize bank client from provided config
	bankClient := bankclient.NewClient(bankclient.Config{
		BaseURL:    appConfig.Bank.BaseURL,
		Timeout:    appConfig.Bank.Timeout,
		MaxRetries: appConfig.Bank.MaxRetries,
	}, zap.L())

	// Initialize services
	paymentService := service.NewPaymentService(db, paymentRepo, bankClient)

	// Initialize handlers
	srv := handlers.NewServer(db, paymentService)

	// Public routes (manual registration for readiness)
	r.GET("/ready", srv.GetReadiness)

	if cfg.Environment != "production" {
		api.RegisterDocsRoutes(r)
	}

	// Wire the idempotency middleware into the generated server so it runs
	// for every route registered by RegisterHandlersWithOptions, before the
	// handler is invoked. The middleware itself already guards on HTTP method,
	// so safe read-only routes (GET /health) are left untouched.
	idempotencyMW := api.MiddlewareFunc(middleware.Idempotency(idempotencyRepo))

	// API routes - use generated OpenAPI router registration
	// This registers: POST /api/v1/payments/authorize and GET /health
	api.RegisterHandlersWithOptions(r, srv, api.GinServerOptions{
		Middlewares: []api.MiddlewareFunc{idempotencyMW},
	})

	return r
}
