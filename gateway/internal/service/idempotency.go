package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/repository"
	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

// IdempotentResult tells the handler exactly what to send to the client.
//
// The service owns the idempotency ledger: the body marshaled here is
// finalized into the idempotency_keys table inside the same transaction that
// commits the payment row, and the handler writes those exact bytes. The
// original response and any replay are therefore byte-for-byte identical and
// the ledger can never drift from what the client actually saw.
type IdempotentResult struct {
	// Replayed reports that this response was not produced by the current
	// request but restored from a committed idempotency record.
	Replayed bool
	// StatusCode is the HTTP status the handler should write.
	StatusCode int
	// Body is the exact response body the handler should write.
	Body []byte
}

const (
	idempotencyStatusInProgress = "in_progress"
	idempotencyStatusComplete   = "complete"
)

// idempotencyScope carries the gateway idempotency coordinates stamped by the
// middleware, extracted once per request.
type idempotencyScope struct {
	key      string
	path     string
	bodyHash string
}

func scopeFromAuthorize(req *models.AuthorizePaymentRequest) idempotencyScope {
	return idempotencyScope{
		key:      req.GatewayIdempotencyKey,
		path:     req.GatewayIdempotencyPath,
		bodyHash: req.GatewayIdempotencyBodyHash,
	}
}

func scopeFromCapture(req *models.CapturePaymentRequest) idempotencyScope {
	return idempotencyScope{
		key:      req.GatewayIdempotencyKey,
		path:     req.GatewayIdempotencyPath,
		bodyHash: req.GatewayIdempotencyBodyHash,
	}
}

func scopeFromRefund(req *models.RefundPaymentRequest) idempotencyScope {
	return idempotencyScope{
		key:      req.GatewayIdempotencyKey,
		path:     req.GatewayIdempotencyPath,
		bodyHash: req.GatewayIdempotencyBodyHash,
	}
}

// claimIdempotency reserves the (key, path) slot inside the caller's open
// payment transaction. Outcomes:
//
//	(nil, nil)     — this transaction owns a fresh in_progress slot; the
//	                 caller must finalize it before committing.
//	(result, nil)  — a committed record already holds the slot; the caller
//	                 must respond with result instead of processing the
//	                 request (replay, or a 409 conflict).
//	(nil, err)     — infrastructure failure; the caller rolls back.
func claimIdempotency(ctx context.Context, repo *repository.Repository, scope idempotencyScope) (*IdempotentResult, error) {
	err := repo.ReserveIdempotencyKey(ctx, scope.key, scope.path, scope.bodyHash)
	if err == nil {
		return nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, NewInternalError(fmt.Errorf("reserve idempotency slot: %w", err))
	}

	// The slot is held by a committed record. A concurrent duplicate blocks on
	// our INSERT until the owning transaction settles, then sees it either
	// complete (replayable) or in_progress (a stale record left by the former
	// middleware-owned flow).
	existing, getErr := repo.GetIdempotencyKey(ctx, scope.key, scope.path)
	if getErr != nil {
		return nil, NewInternalError(fmt.Errorf("load idempotency record: %w", getErr))
	}

	return resolveIdempotencyConflict(&existing, scope.bodyHash), nil
}

// resolveIdempotencyConflict decides the response when a committed record
// already holds the (key, path) slot. Pure so it can be tested without a DB.
func resolveIdempotencyConflict(existing *store.IdempotencyKey, requestHash string) *IdempotentResult {
	if existing.RequestHash != requestHash {
		zap.L().Warn(
			"idempotency key reused with different request body",
			zap.String("idempotency_key", existing.Key),
			zap.String("path", existing.RequestPath),
		)
		return &IdempotentResult{
			StatusCode: http.StatusConflict,
			Body: idempotencyErrorBody(
				"idempotency_key_conflict",
				"Idempotency-Key has been used with a different request body",
			),
		}
	}

	if existing.Status != idempotencyStatusComplete {
		zap.L().Warn(
			"idempotency key already in progress",
			zap.String("idempotency_key", existing.Key),
			zap.String("path", existing.RequestPath),
		)
		return &IdempotentResult{
			StatusCode: http.StatusConflict,
			Body: idempotencyErrorBody(
				"idempotency_key_in_progress",
				"A request with this Idempotency-Key is already being processed",
			),
		}
	}

	// The slot is complete — the committed outcome already exists, replay it
	// verbatim so the client reliably learns what happened, whatever the
	// recorded status is.
	return &IdempotentResult{
		Replayed:   true,
		StatusCode: int(existing.ResponseCode.Int32),
		Body:       append(json.RawMessage(nil), existing.ResponseBody...),
	}
}

// idempotencyErrorBody marshals the error envelope stored alongside the
// idempotency record. Only plain strings go in, so this cannot fail.
func idempotencyErrorBody(code, message string) []byte {
	body, err := json.Marshal(models.ErrorResponse{Error: code, Message: message, Code: code})
	if err != nil {
		panic(fmt.Sprintf("marshal idempotency error body: %v", err))
	}
	return body
}
