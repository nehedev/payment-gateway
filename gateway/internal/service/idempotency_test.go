package service

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/nehemiah-dev/payment-gateway/internal/models"
	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

func completeRecord(body string) store.IdempotencyKey {
	return store.IdempotencyKey{
		ID:           1,
		Key:          "test-key",
		RequestPath:  "/api/v1/payments/authorize",
		RequestHash:  "body-hash-1",
		ResponseCode: pgtype.Int4{Int32: http.StatusOK, Valid: true},
		ResponseBody: json.RawMessage(body),
		Status:       "complete",
		CreatedAt:    time.Now(),
	}
}

const recordedBody = `{"payment_id":"1"}`

func TestResolveIdempotencyConflictReplaysCommittedRecord(t *testing.T) {
	record := completeRecord(recordedBody)

	result := resolveIdempotencyConflict(&record, "body-hash-1")

	require.True(t, result.Replayed)
	require.Equal(t, http.StatusOK, result.StatusCode)
	require.JSONEq(t, recordedBody, string(result.Body))
}

func TestResolveIdempotencyConflictHashMismatch(t *testing.T) {
	record := completeRecord(recordedBody)

	result := resolveIdempotencyConflict(&record, "body-hash-2")

	require.False(t, result.Replayed)
	require.Equal(t, http.StatusConflict, result.StatusCode)

	var payload models.ErrorResponse
	require.NoError(t, json.Unmarshal(result.Body, &payload))
	require.Equal(t, "idempotency_key_conflict", payload.Error)
	require.Equal(t, "idempotency_key_conflict", payload.Code)
}

func TestResolveIdempotencyConflictInProgressBlocksWithoutReplay(t *testing.T) {
	// A committed in_progress row is a stale record from the former
	// middleware-owned flow — clients must back off, not get a bogus replay.
	record := completeRecord(recordedBody)
	record.Status = "in_progress"
	record.ResponseCode = pgtype.Int4{}
	record.ResponseBody = nil

	result := resolveIdempotencyConflict(&record, "body-hash-1")

	require.False(t, result.Replayed)
	require.Equal(t, http.StatusConflict, result.StatusCode)

	var payload models.ErrorResponse
	require.NoError(t, json.Unmarshal(result.Body, &payload))
	require.Equal(t, "idempotency_key_in_progress", payload.Error)
}

func TestResolveIdempotencyConflictReplaysCommittedErrorOutcome(t *testing.T) {
	// Declines are recorded as complete records with mapped error codes —
	// replaying them is what lets a client learn the transaction failed.
	record := completeRecord(`{"error":"insufficient_funds","message":"declined","code":"insufficient_funds"}`)
	record.ResponseCode = pgtype.Int4{Int32: http.StatusPaymentRequired, Valid: true}

	result := resolveIdempotencyConflict(&record, "body-hash-1")

	require.True(t, result.Replayed)
	require.Equal(t, http.StatusPaymentRequired, result.StatusCode)
	require.Contains(t, string(result.Body), "insufficient_funds")
}

func TestIdempotencyErrorBodyShape(t *testing.T) {
	body := idempotencyErrorBody("some_code", "some message")

	var payload models.ErrorResponse
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, "some_code", payload.Error)
	require.Equal(t, "some message", payload.Message)
	require.Equal(t, "some_code", payload.Code)
}
