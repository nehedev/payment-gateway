package router

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/database"
	"github.com/nehemiah-dev/payment-gateway/internal/handlers"
	"github.com/nehemiah-dev/payment-gateway/internal/middleware"
)

type Config struct {
	Environment    string
	AllowedOrigins []string
}

func New(db *database.DB, cfg Config) *gin.Engine {
	r := gin.New()

	r.Use(middleware.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	r.Use(middleware.RequestID())
	r.Use(middleware.Logger())

	srv := handlers.NewServer(db)

	r.GET("/health", srv.GetHealth)
	r.GET("/ready", srv.GetReadiness)

	if cfg.Environment != "production" {
		api.RegisterDocsRoutes(r)
	}

	v1 := r.Group("/api/v1")
	v1.GET("/health", srv.GetHealth)

	return r
}
