package bankclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Gateway interface {
	Authorize(ctx context.Context, req AuthorizeRequest) (*AuthorizeResult, error)
	Capture(ctx context.Context, req CaptureRequest) (*CaptureResult, error)
	Void(ctx context.Context, req VoidRequest) (*VoidResult, error)
	Refund(ctx context.Context, req RefundRequest) (*RefundResult, error)
	GetAuthorization(ctx context.Context, authorizationID string) (*AuthorizeResult, error)
	GetCapture(ctx context.Context, captureID string) (*CaptureResult, error)
	GetRefund(ctx context.Context, refundID string) (*RefundResult, error)
}

var _ Gateway = (*Client)(nil)

func (c *Client) Authorize(ctx context.Context, req AuthorizeRequest) (*AuthorizeResult, error) {
	wireReq := authorizeWireRequest{
		CardNumber:  req.CardNumber,
		CVV:         req.CVV,
		ExpiryMonth: req.ExpiryMonth,
		ExpiryYear:  req.ExpiryYear,
		Amount:      req.AmountCents,
	}
	raw, err := c.doRequest(ctx, http.MethodPost, "/api/v1/authorizations", wireReq, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	var wire authorizationWireResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal authorize response: %w", err)
	}
	return &AuthorizeResult{
		AuthorizationID: wire.AuthorizationID,
		Status:          wire.Status,
		AmountCents:     wire.Amount,
		Currency:        wire.Currency,
		ExpiresAt:       wire.ExpiresAt,
		CreatedAt:       wire.CreatedAt,
	}, nil
}

func (c *Client) GetAuthorization(ctx context.Context, authorizationID string) (*AuthorizeResult, error) {
	raw, err := c.doRequest(ctx, http.MethodGet, "/api/v1/authorizations/"+authorizationID, nil, "")
	if err != nil {
		return nil, err
	}
	var wire authorizationWireResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal authorization: %w", err)
	}
	return &AuthorizeResult{
		AuthorizationID: wire.AuthorizationID,
		Status:          wire.Status,
		AmountCents:     wire.Amount,
		Currency:        wire.Currency,
		ExpiresAt:       wire.ExpiresAt,
		CreatedAt:       wire.CreatedAt,
	}, nil
}

func (c *Client) Capture(ctx context.Context, req CaptureRequest) (*CaptureResult, error) {
	wireReq := captureWireRequest{AuthorizationID: req.AuthorizationID, Amount: req.AmountCents}
	raw, err := c.doRequest(ctx, http.MethodPost, "/api/v1/captures", wireReq, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	var wire captureWireResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal capture response: %w", err)
	}
	return &CaptureResult{
		CaptureID:       wire.CaptureID,
		AuthorizationID: wire.AuthorizationID,
		Status:          wire.Status,
		AmountCents:     wire.Amount,
		Currency:        wire.Currency,
		CapturedAt:      wire.CapturedAt,
	}, nil
}

func (c *Client) GetCapture(ctx context.Context, captureID string) (*CaptureResult, error) {
	raw, err := c.doRequest(ctx, http.MethodGet, "/api/v1/captures/"+captureID, nil, "")
	if err != nil {
		return nil, err
	}
	var wire captureWireResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal capture: %w", err)
	}
	return &CaptureResult{
		CaptureID:       wire.CaptureID,
		AuthorizationID: wire.AuthorizationID,
		Status:          wire.Status,
		AmountCents:     wire.Amount,
		Currency:        wire.Currency,
		CapturedAt:      wire.CapturedAt,
	}, nil
}

func (c *Client) Void(ctx context.Context, req VoidRequest) (*VoidResult, error) {
	wireReq := voidWireRequest{AuthorizationID: req.AuthorizationID}
	raw, err := c.doRequest(ctx, http.MethodPost, "/api/v1/voids", wireReq, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	var wire voidWireResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal void response: %w", err)
	}
	return &VoidResult{
		VoidID:          wire.VoidID,
		AuthorizationID: wire.AuthorizationID,
		Status:          wire.Status,
		VoidedAt:        wire.VoidedAt,
	}, nil
}

// No GetVoid — the spec doesn't expose a GET /voids/{id} endpoint.

func (c *Client) Refund(ctx context.Context, req RefundRequest) (*RefundResult, error) {
	wireReq := refundWireRequest{CaptureID: req.CaptureID, Amount: req.AmountCents}
	raw, err := c.doRequest(ctx, http.MethodPost, "/api/v1/refunds", wireReq, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	var wire refundWireResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal refund response: %w", err)
	}
	return &RefundResult{
		RefundID:    wire.RefundID,
		CaptureID:   wire.CaptureID,
		Status:      wire.Status,
		AmountCents: wire.Amount,
		Currency:    wire.Currency,
		RefundedAt:  wire.RefundedAt,
	}, nil
}

func (c *Client) GetRefund(ctx context.Context, refundID string) (*RefundResult, error) {
	raw, err := c.doRequest(ctx, http.MethodGet, "/api/v1/refunds/"+refundID, nil, "")
	if err != nil {
		return nil, err
	}
	var wire refundWireResponse
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("unmarshal refund: %w", err)
	}
	return &RefundResult{
		RefundID:    wire.RefundID,
		CaptureID:   wire.CaptureID,
		Status:      wire.Status,
		AmountCents: wire.Amount,
		Currency:    wire.Currency,
		RefundedAt:  wire.RefundedAt,
	}, nil
}
