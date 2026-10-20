package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	IdempotencyKeyHeader = "Idempotency-Key"
	ReplayedHeader       = "X-Idempotent-Replayed"

	// Gin context keys stamped for the downstream handlers. The service layer
	// reads them to reserve and finalize the idempotency record inside the
	// payment transaction.
	CtxIdempotencyKey      = "idempotency_key"
	CtxIdempotencyPath     = "idempotency_path"
	CtxIdempotencyBodyHash = "idempotency_body_hash"
)

// Idempotency validates the client key and stamps the request coordinates
// (key, request path, SHA-256 of the raw body) into the request context.
//
// It performs no database work by design: ownership of the idempotency record
// belongs to the payment service, which reserves and finalizes the record
// inside the same transaction that commits the payment row. The payment
// outcome and its idempotency record therefore commit or roll back atomically:
// whatever the client observes above 2xx is either a definitive, replayable
// committed result (declines included) or a clean rollback that may be retried
// with the same key.
func Idempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost &&
			c.Request.Method != http.MethodPut &&
			c.Request.Method != http.MethodPatch &&
			c.Request.Method != http.MethodDelete {
			c.Next()
			return
		}

		idempotencyKey := c.GetHeader(IdempotencyKeyHeader)
		if idempotencyKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "missing_idempotency_key",
				"message": "Idempotency-Key header is required for write operations",
			})
			c.Abort()
			return
		}

		if len(idempotencyKey) > 255 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_idempotency_key",
				"message": "Idempotency-Key must be 255 characters or less",
			})
			c.Abort()
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			zap.L().Error("failed to read request body", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "Failed to process request",
			})
			c.Abort()
			return
		}

		// Restore the body for downstream handlers.
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		hash := sha256.Sum256(bodyBytes)

		// Keys are scoped to the request path so the same UUID cannot
		// accidentally replay across different endpoints.
		c.Set(CtxIdempotencyKey, idempotencyKey)
		c.Set(CtxIdempotencyPath, c.Request.URL.Path)
		c.Set(CtxIdempotencyBodyHash, hex.EncodeToString(hash[:]))

		c.Next()
	}
}
