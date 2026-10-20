package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/nehemiah-dev/payment-gateway/internal/api"
	"github.com/nehemiah-dev/payment-gateway/internal/middleware"
	"github.com/nehemiah-dev/payment-gateway/internal/repository"
)

type memoryIdempotencyRepository struct {
	mu      sync.Mutex
	records map[string]*repository.IdempotencyRecord
}

func newMemoryIdempotencyRepository() *memoryIdempotencyRepository {
	return &memoryIdempotencyRepository{records: make(map[string]*repository.IdempotencyRecord)}
}

func (r *memoryIdempotencyRepository) Get(_ context.Context, key, requestPath string) (*repository.IdempotencyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if record := r.records[key+"\x00"+requestPath]; record != nil {
		copiedRecord := *record
		return &copiedRecord, nil
	}
	return nil, nil
}

func (r *memoryIdempotencyRepository) Reserve(_ context.Context, key, requestPath, requestHash string) (*repository.IdempotencyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	storageKey := key + "\x00" + requestPath
	if r.records[storageKey] != nil {
		return nil, nil
	}
	record := &repository.IdempotencyRecord{
		Key:         key,
		RequestPath: requestPath,
		RequestHash: requestHash,
		Status:      "in_progress",
	}
	r.records[storageKey] = record
	return record, nil
}

func (r *memoryIdempotencyRepository) Finalize(_ context.Context, key, requestPath string, statusCode int, body json.RawMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record := r.records[key+"\x00"+requestPath]
	record.Status = "complete"
	record.ResponseCode = statusCode
	record.ResponseBody = append(json.RawMessage(nil), body...)
	return nil
}

func (r *memoryIdempotencyRepository) Release(_ context.Context, key, requestPath string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, key+"\x00"+requestPath)
	return nil
}

type idempotencyTestServer struct {
	calls    int
	statuses []int
}

func (s *idempotencyTestServer) AuthorizePayment(c *gin.Context, _ api.AuthorizePaymentParams) {
	s.calls++
	status := http.StatusCreated
	if len(s.statuses) >= s.calls {
		status = s.statuses[s.calls-1]
	}
	c.JSON(status, gin.H{"attempt": s.calls})
}

func (s *idempotencyTestServer) GetHealth(c *gin.Context) {
	c.Status(http.StatusOK)
}

func TestGeneratedAuthorizeRouteReplaysCachedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newMemoryIdempotencyRepository()
	server := &idempotencyTestServer{}
	router := gin.New()

	registerAPIHandlers(router, server, repo)

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/authorize", strings.NewReader(`{"amount":100}`))
		req.Header.Set("Idempotency-Key", "test-key")
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	first := request()
	second := request()

	require.Equal(t, http.StatusCreated, first.Code)
	require.Equal(t, first.Body.String(), second.Body.String())
	require.Equal(t, http.StatusCreated, second.Code)
	require.Equal(t, 1, server.calls)
	require.Equal(t, "true", second.Header().Get(middleware.ReplayedHeader))
}

func TestFailedGeneratedAuthorizeRequestReleasesIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newMemoryIdempotencyRepository()
	server := &idempotencyTestServer{statuses: []int{http.StatusInternalServerError, http.StatusCreated}}
	router := gin.New()
	registerAPIHandlers(router, server, repo)

	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/authorize", strings.NewReader(`{"amount":100}`))
		req.Header.Set("Idempotency-Key", "retry-key")
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	first := request()
	second := request()

	require.Equal(t, http.StatusInternalServerError, first.Code)
	require.Equal(t, http.StatusCreated, second.Code)
	require.Equal(t, 2, server.calls)
}
