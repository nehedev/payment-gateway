package bankclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Config struct {
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
}

type Client struct {
	httpClient *http.Client
	baseURL    string
	maxRetries int
	logger     *zap.Logger
}

func NewClient(cfg Config, logger *zap.Logger) *Client {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		baseURL:    cfg.BaseURL,
		maxRetries: cfg.MaxRetries,
		logger:     logger,
	}
}

// doRequest is the single choke point every operation funnels through.
// idempotencyKey is only sent when non-empty (GET requests pass "").
// NEVER log reqBody here — for authorize calls it contains raw card_number/cvv.
func (c *Client) doRequest(ctx context.Context, method, path string, reqBody any, idempotencyKey string) ([]byte, error) {
	var bodyBytes []byte
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyBytes = b
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * 100 * time.Millisecond
			jitter := time.Duration(rand.Int63n(int64(backoff)/2 + 1))
			select {
			case <-time.After(backoff + jitter):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		start := time.Now()
		resp, err := c.httpClient.Do(req)
		dur := time.Since(start)

		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			c.logger.Warn("bank request transport error, will retry",
				zap.Error(err), zap.Int("attempt", attempt), zap.Duration("duration", dur),
				zap.String("method", method), zap.String("path", path))
			continue
		}

		respData, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read response: %w", readErr)
			continue
		}

		c.logger.Debug("bank request completed",
			zap.String("method", method), zap.String("path", path),
			zap.Int("status", resp.StatusCode), zap.Duration("duration", dur), zap.Int("attempt", attempt))

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respData, nil
		}

		bankErr := parseErrorResponse(resp.StatusCode, respData)
		lastErr = bankErr

		if !bankErr.Retryable {
			return nil, bankErr
		}
		c.logger.Warn("bank returned retryable error (likely the injected 5% failure)",
			zap.Int("status", resp.StatusCode), zap.Int("attempt", attempt))
	}

	return nil, fmt.Errorf("bank request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}