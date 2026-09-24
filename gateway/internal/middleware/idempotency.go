package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/repository"
)

const (
	IdempotencyKeyHeader = "Idempotency-Key"
	ReplayedHeader       = "X-Idempotent-Replayed"
)

// responseRecorder wraps gin.ResponseWriter to capture response data
type responseRecorder struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// Idempotency middleware enforces idempotent semantics on write operations.
// For a given (Idempotency-Key, path) pair it returns the original cached
// response on replay rather than re-processing the request. Keys are scoped
// to the request path so the same UUID cannot accidentally replay across
// different endpoints.
func Idempotency(repo repository.IdempotencyRepository) gin.HandlerFunc {
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

		requestPath := c.Request.URL.Path

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
		requestHash := hex.EncodeToString(hash[:])

		ctx := c.Request.Context()
		existing, err := repo.Get(ctx, idempotencyKey, requestPath)
		if err != nil {
			zap.L().Error(
				"failed to check idempotency key",
				zap.Error(err),
				zap.String("idempotency_key", idempotencyKey),
				zap.String("path", requestPath),
			)
			// Don't block the request on a store read failure — idempotency
			// is a best-effort safety net, not a hard dependency.
			c.Next()
			return
		}

		if existing != nil {
			// Same key, different body — the client is misusing the key.
			if existing.RequestHash != requestHash {
				zap.L().Warn(
					"idempotency key reused with different request body",
					zap.String("idempotency_key", idempotencyKey),
					zap.String("path", requestPath),
				)
				c.JSON(http.StatusConflict, gin.H{
					"error":   "idempotency_key_conflict",
					"message": "Idempotency-Key has been used with a different request body",
				})
				c.Abort()
				return
			}

			c.Header(ReplayedHeader, "true")

			var responseBody interface{}
			if err := json.Unmarshal(existing.ResponseBody, &responseBody); err != nil {
				zap.L().Error(
					"failed to unmarshal cached response",
					zap.Error(err),
					zap.String("idempotency_key", idempotencyKey),
				)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "internal_error",
					"message": "Failed to replay cached response",
				})
				c.Abort()
				return
			}

			zap.L().Info(
				"replaying cached response",
				zap.String("idempotency_key", idempotencyKey),
				zap.String("path", requestPath),
				zap.Int("status_code", existing.ResponseCode),
			)

			c.JSON(existing.ResponseCode, responseBody)
			c.Abort()
			return
		}

		recorder := &responseRecorder{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
			statusCode:     http.StatusOK,
		}
		c.Writer = recorder

		c.Next()

		// Only cache successful responses; errors are not idempotent by design.
		if recorder.statusCode >= 200 && recorder.statusCode < 300 {
			record := &repository.IdempotencyRecord{
				Key:          idempotencyKey,
				RequestPath:  requestPath,
				RequestHash:  requestHash,
				ResponseCode: recorder.statusCode,
				ResponseBody: json.RawMessage(recorder.body.Bytes()),
			}

			if err := repo.Store(ctx, record); err != nil {
				// Log but don't fail — the response has already been sent.
				zap.L().Error(
					"failed to store idempotency record",
					zap.Error(err),
					zap.String("idempotency_key", idempotencyKey),
					zap.String("path", requestPath),
				)
			} else {
				zap.L().Debug(
					"stored idempotency record",
					zap.String("idempotency_key", idempotencyKey),
					zap.String("path", requestPath),
					zap.Int("status_code", recorder.statusCode),
				)
			}
		}
	}
}
