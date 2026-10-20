package router

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/middleware"
)

// coordinates captures the idempotency values stamped into the request context.
type coordinates struct {
	calls    int
	key      string
	path     string
	bodyHash string
}

func (s *coordinates) AuthorizePayment(c *gin.Context, _ api.AuthorizePaymentParams) {
	s.calls++
	s.key = c.GetString(middleware.CtxIdempotencyKey)
	s.path = c.GetString(middleware.CtxIdempotencyPath)
	s.bodyHash = c.GetString(middleware.CtxIdempotencyBodyHash)
	c.Status(http.StatusOK)
}

func (s *coordinates) CapturePayment(c *gin.Context, _ openapi_types.UUID, _ api.CapturePaymentParams) {
	s.calls++
	s.key = c.GetString(middleware.CtxIdempotencyKey)
	s.path = c.GetString(middleware.CtxIdempotencyPath)
	s.bodyHash = c.GetString(middleware.CtxIdempotencyBodyHash)
	c.Status(http.StatusOK)
}

func (s *coordinates) RefundPayment(c *gin.Context, _ openapi_types.UUID, _ api.RefundPaymentParams) {
	s.calls++
	s.key = c.GetString(middleware.CtxIdempotencyKey)
	s.path = c.GetString(middleware.CtxIdempotencyPath)
	s.bodyHash = c.GetString(middleware.CtxIdempotencyBodyHash)
	c.Status(http.StatusOK)
}

func (s *coordinates) VoidPayment(c *gin.Context, _ openapi_types.UUID, _ api.VoidPaymentParams) {
	s.calls++
	s.key = c.GetString(middleware.CtxIdempotencyKey)
	s.path = c.GetString(middleware.CtxIdempotencyPath)
	s.bodyHash = c.GetString(middleware.CtxIdempotencyBodyHash)
	c.Status(http.StatusOK)
}

func (s *coordinates) GetPayment(c *gin.Context, _ openapi_types.UUID) {
	c.Status(http.StatusOK)
}

func (s *coordinates) ListPayments(c *gin.Context, _ api.ListPaymentsParams) {
	c.Status(http.StatusOK)
}

func (s *coordinates) GetHealth(c *gin.Context) {
	c.Status(http.StatusOK)
}

func newTestRouter(server api.ServerInterface) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerAPIHandlers(router, server)
	return router
}

func serveAuthorize(t *testing.T, router *gin.Engine, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/authorize", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestIdempotencyMiddlewareRejectsMissingKey(t *testing.T) {
	server := &coordinates{}
	router := newTestRouter(server)

	res := serveAuthorize(t, router, "", `{"amount":100}`)

	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "missing_idempotency_key")
	require.Zero(t, server.calls)
}

func TestIdempotencyMiddlewareRejectsOversizedKey(t *testing.T) {
	server := &coordinates{}
	router := newTestRouter(server)

	res := serveAuthorize(t, router, strings.Repeat("k", 256), `{"amount":100}`)

	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "invalid_idempotency_key")
	require.Zero(t, server.calls)
}

func TestIdempotencyMiddlewareStampsRequestCoordinates(t *testing.T) {
	server := &coordinates{}
	router := newTestRouter(server)

	body := `{"amount":100}`
	res := serveAuthorize(t, router, "test-key", body)

	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, 1, server.calls)
	require.Equal(t, "test-key", server.key)
	require.Equal(t, "/api/v1/payments/authorize", server.path)

	hash := sha256.Sum256([]byte(body))
	require.Equal(t, hex.EncodeToString(hash[:]), server.bodyHash)
}

func TestIdempotencyMiddlewareSkipsReads(t *testing.T) {
	server := &coordinates{}
	router := newTestRouter(server)

	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestIdempotencyMiddlewareStampsCaptureCoordinates(t *testing.T) {
	server := &coordinates{}
	router := newTestRouter(server)

	body := `{"amount":5000}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/payments/550e8400-e29b-41d4-a716-446655440000/capture",
		strings.NewReader(body))
	req.Header.Set("Idempotency-Key", "cap-key")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, server.calls)
	require.Equal(t, "cap-key", server.key)
	require.Equal(t, "/api/v1/payments/550e8400-e29b-41d4-a716-446655440000/capture", server.path)

	hash := sha256.Sum256([]byte(body))
	require.Equal(t, hex.EncodeToString(hash[:]), server.bodyHash)
}
