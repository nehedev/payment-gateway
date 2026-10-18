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

	statusInProgress = "in_progress"
	statusComplete   = "complete"
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
//
// For a given (Idempotency-Key, path) pair the middleware guarantees:
//   - Missing key            → 400
//   - Same key + diff body   → 409 Conflict
//   - Same key + same body   → cached response + X-Idempotent-Replayed: true
//   - Concurrent duplicate   → 409 (second request loses the Reserve race)
//
// Lifecycle of a slot:
//  1. Reserve: INSERT ... ON CONFLICT DO NOTHING — winner gets the row,
//     loser gets nil (returns 409 immediately).
//  2. Process: downstream handler runs normally inside the winner.
//  3. Finalize: UPDATE the row with the response (status → complete).
//
// Keys are scoped to the request path so the same UUID cannot accidentally
// replay across different endpoints.
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

		// Check for an existing record first (replay or in-progress).
		existing, err := repo.Get(ctx, idempotencyKey, requestPath)
		if err != nil {
			zap.L().Error(
				"failed to check idempotency key",
				zap.Error(err),
				zap.String("idempotency_key", idempotencyKey),
				zap.String("path", requestPath),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "Failed to process request",
			})
			c.Abort()
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

			if existing.Status == statusInProgress {
				// Another request is already processing this key — tell the
				// caller to back off and retry once the original finishes.
				zap.L().Warn(
					"idempotency key already in progress",
					zap.String("idempotency_key", idempotencyKey),
					zap.String("path", requestPath),
				)
				c.JSON(http.StatusConflict, gin.H{
					"error":   "idempotency_key_in_progress",
					"message": "A request with this Idempotency-Key is already being processed",
				})
				c.Abort()
				return
			}

			// Status is complete — replay the cached response.
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

			c.Header(ReplayedHeader, "true")
			c.JSON(existing.ResponseCode, responseBody)
			c.Abort()
			return
		}

		// No existing record — atomically claim the slot before processing.
		// If Reserve returns nil the unique constraint was hit: a concurrent
		// request already owns this slot.
		reserved, err := repo.Reserve(ctx, idempotencyKey, requestPath, requestHash)
		if err != nil {
			zap.L().Error(
				"failed to reserve idempotency slot",
				zap.Error(err),
				zap.String("idempotency_key", idempotencyKey),
				zap.String("path", requestPath),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "internal_error",
				"message": "Failed to process request",
			})
			c.Abort()
			return
		}

		if reserved == nil {
			// Lost the reserve race — another goroutine/process beat us.
			zap.L().Warn(
				"idempotency reserve conflict",
				zap.String("idempotency_key", idempotencyKey),
				zap.String("path", requestPath),
			)
			c.JSON(http.StatusConflict, gin.H{
				"error":   "idempotency_key_in_progress",
				"message": "A request with this Idempotency-Key is already being processed",
			})
			c.Abort()
			return
		}

		// We own the slot — wrap the writer to capture the response.
		recorder := &responseRecorder{
			ResponseWriter: c.Writer,
			body:           bytes.NewBuffer(nil),
			statusCode:     http.StatusOK,
		}
		c.Writer = recorder

		c.Next()

		// Finalize: persist the response so future requests can replay it.
		// Only cache successful responses; errors are not idempotent by design
		// (a retry after a 5xx should be allowed to re-attempt processing).
		if recorder.statusCode >= 200 && recorder.statusCode < 300 {
			if err := repo.Finalize(ctx, idempotencyKey, requestPath, recorder.statusCode, json.RawMessage(recorder.body.Bytes())); err != nil {
				// Log but don't fail — the response has already been sent and
				// the client received a success. The in_progress slot will stay
				// and subsequent retries will receive a 409 until cleaned up.
				zap.L().Error(
					"failed to finalize idempotency record",
					zap.Error(err),
					zap.String("idempotency_key", idempotencyKey),
					zap.String("path", requestPath),
				)
			} else {
				zap.L().Debug(
					"finalized idempotency record",
					zap.String("idempotency_key", idempotencyKey),
					zap.String("path", requestPath),
					zap.Int("status_code", recorder.statusCode),
				)
			}
		}
	}
}
