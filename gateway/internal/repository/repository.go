package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nehemiah-dev/payment-gateway/internal/store"
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *store.Queries
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: store.New(pool),
	}
}

// --- PAYMENT WRAPPER METHODS ---

func (r *Repository) CreatePayment(ctx context.Context, arg *store.CreatePaymentParams) (store.Payment, error) {
	return r.queries.CreatePayment(ctx, *arg)
}

func (r Repository) GetPaymentByID(ctx context.Context, id int) (store.Payment, error) {
	return r.queries.GetPaymentByID(ctx, int64(id))
}

func (r Repository) GetPaymentByRef(ctx context.Context, ref uuid.UUID) (store.Payment, error) {
	return r.queries.GetPaymentByReference(ctx, ref)
}

func (r *Repository) AuthorizePayment(ctx context.Context, arg store.AuthorizePaymentParams) (store.Payment, error) {
	return r.queries.AuthorizePayment(ctx, arg)
}

func (r *Repository) CapturePayment(ctx context.Context, arg store.CapturePaymentParams) (store.Payment, error) {
	return r.queries.CapturePayment(ctx, arg)
}

func (r *Repository) VoidPayment(ctx context.Context, arg store.VoidPaymentParams) (store.Payment, error) {
	return r.queries.VoidPayment(ctx, arg)
}

func (r *Repository) RefundPayment(ctx context.Context, arg store.RefundPaymentParams) (store.Payment, error) {
	return r.queries.RefundPayment(ctx, arg)
}

func (r *Repository) ListPaymentsByCustomer(ctx context.Context, arg store.ListPaymentsByCustomerParams) ([]store.Payment, error) {
	return r.queries.ListPaymentsByCustomer(ctx, arg)
}

func (r *Repository) ListPaymentsByOrder(ctx context.Context, orderID string) ([]store.Payment, error) {
	return r.queries.ListPaymentsByOrder(ctx, orderID)
}

func (r *Repository) FailPayment(ctx context.Context, paymentID int64) (store.Payment, error) {
	return r.queries.FailPayment(ctx, paymentID)
}

// --- IDEMPOTENCY ---

// NewTxRepository binds a Repository to an open transaction. The Querier
// handed to database.WithTx callbacks satisfies store.DBTX, so queries built
// from it execute on the transaction instead of the pool. That is what lets
// the payment service commit the payment row and the idempotency ledger row
// together — a rollback discards both.
func NewTxRepository(txQuerier store.DBTX) *Repository {
	return &Repository{queries: store.New(txQuerier)}
}

// ReserveIdempotencyKey atomically claims an in_progress slot for
// (key, requestPath). Returns pgx.ErrNoRows when a committed record already
// holds the slot (conflict) — the caller must then resolve the existing
// record instead of processing the request.
func (r *Repository) ReserveIdempotencyKey(ctx context.Context, key, requestPath, requestHash string) error {
	_, err := r.queries.ReserveIdempotencyKey(ctx, store.ReserveIdempotencyKeyParams{
		Key:         key,
		RequestPath: requestPath,
		RequestHash: requestHash,
	})
	// ON CONFLICT DO NOTHING with a :one query surfaces pgx.ErrNoRows when
	// the INSERT was suppressed — a committed record already owns the slot.
	return err
}

// GetIdempotencyKey returns the ledger record for (key, requestPath), or
// pgx.ErrNoRows when none exists.
func (r *Repository) GetIdempotencyKey(ctx context.Context, key, requestPath string) (store.IdempotencyKey, error) {
	return r.queries.GetIdempotencyKey(ctx, store.GetIdempotencyKeyParams{
		Key:         key,
		RequestPath: requestPath,
	})
}

// FinalizeIdempotencyKey records the final response for the slot owned by the
// current transaction and flips its status to complete. It must run inside the
// same transaction that commits the payment row so that a committed payment is
// always replayable.
func (r *Repository) FinalizeIdempotencyKey(ctx context.Context, key, requestPath string, statusCode int, body json.RawMessage) error {
	if statusCode < 100 || statusCode > 599 {
		return fmt.Errorf("invalid HTTP response code: %d", statusCode)
	}
	_, err := r.queries.FinalizeIdempotencyKey(ctx, store.FinalizeIdempotencyKeyParams{
		Key:          key,
		RequestPath:  requestPath,
		ResponseCode: int32(statusCode),
		ResponseBody: body,
	})
	return err
}
