package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/config"
	"github.com/nehemiah-dev/payment-gateway/internal/database"
	"github.com/nehemiah-dev/payment-gateway/internal/logger"
	"github.com/nehemiah-dev/payment-gateway/internal/router"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("APP_ENV") != "production" {
		if err := godotenv.Load(); err != nil {
			fmt.Fprintln(os.Stderr, "no .env file found, relying on system env vars")
		}
	}

	logger.Init()
	defer logger.Sync()

	cfg, err := config.Load()
	if err != nil {
		zap.L().Error("failed to load config", zap.Error(err))
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Raw pool
	pool, err := database.NewPool(ctx, database.PoolConfig{
		DSN:             cfg.Database.DSN(),
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
	})
	if err != nil {
		zap.L().Error("failed to connect to database", zap.Error(err))
		return fmt.Errorf("database connection: %w", err)
	}

	// Wrap the pool: this is what gets threaded through the app from here on —
	// every query and transaction, everywhere, goes through dbConn.
	dbConn := database.New(pool, zap.L())
	defer dbConn.Close() // closes the underlying pool

	zap.L().Info("connected to database")

	r := router.New(dbConn, router.Config{
		AllowedOrigins: cfg.Server.AllowedOrigins,
		Environment:    cfg.AppEnv,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		zap.L().Info("starting server", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	select {
	case <-ctx.Done():
		zap.L().Info("shutdown signal received")
	case err := <-serverErrCh:
		if err != nil {
			zap.L().Error("server failed", zap.Error(err))
			return fmt.Errorf("server error: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zap.L().Error("graceful shutdown failed", zap.Error(err))
		return fmt.Errorf("server shutdown: %w", err)
	}

	zap.L().Info("shutdown complete")
	return nil
	// dbConn.Close() then logger.Sync() fire here, in that order (LIFO defers)
}
