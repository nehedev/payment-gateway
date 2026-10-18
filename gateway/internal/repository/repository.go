package repository

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// IdempotencyRecord is the domain representation of a stored idempotency entry.
// It mirrors store.IdempotencyKey but is intentionally kept as a separate type
// so the middleware layer is not coupled to the sqlc-generated store types.
type IdempotencyRecord struct {
	ID           int64
	Key          string
	RequestPath  string
	RequestHash  string
	ResponseCode int
	ResponseBody json.RawMessage
	// Status is "in_progress" while the owning request is being processed,
	// "complete" once the response has been cached.
	Status string
}

// IdempotencyRepository defines the contract used by the idempotency middleware.
type IdempotencyRepository interface {
	// Get returns the cached record for (key, path), or nil if none exists.
	Get(ctx context.Context, key, requestPath string) (*IdempotencyRecord, error)
	// Reserve atomically inserts an in_progress slot for (key, path, hash).
	// Returns (record, nil) if this caller won the race and owns the slot.
	// Returns (nil, nil) if another request already holds the slot (conflict).
	Reserve(ctx context.Context, key, requestPath, requestHash string) (*IdempotencyRecord, error)
	// Finalize updates an in_progress record with the final response.
	Finalize(ctx context.Context, key, requestPath string, statusCode int, body json.RawMessage) error
}

// NewIdempotencyRepository returns an IdempotencyRepository backed by the
// shared sqlc store — the same pattern used for payments.
func NewIdempotencyRepository(pool *pgxpool.Pool) IdempotencyRepository {
	return &idempotencyRepository{queries: store.New(pool)}
}

type idempotencyRepository struct {
	queries *store.Queries
}

func (r *idempotencyRepository) Get(ctx context.Context, key, requestPath string) (*IdempotencyRecord, error) {
	row, err := r.queries.GetIdempotencyKey(ctx, store.GetIdempotencyKeyParams{
		Key:         key,
		RequestPath: requestPath,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	rec := &IdempotencyRecord{
		ID:          row.ID,
		Key:         row.Key,
		RequestPath: row.RequestPath,
		RequestHash: row.RequestHash,
		Status:      row.Status,
	}
	if row.ResponseCode.Valid {
		rec.ResponseCode = int(row.ResponseCode.Int32)
	}
	if row.ResponseBody != nil {
		rec.ResponseBody = row.ResponseBody
	}
	return rec, nil
}

func (r *idempotencyRepository) Reserve(ctx context.Context, key, requestPath, requestHash string) (*IdempotencyRecord, error) {
	row, err := r.queries.ReserveIdempotencyKey(ctx, store.ReserveIdempotencyKeyParams{
		Key:         key,
		RequestPath: requestPath,
		RequestHash: requestHash,
	})
	if err != nil {
		// ON CONFLICT DO NOTHING with a :one query returns pgx.ErrNoRows when
		// the INSERT was suppressed — another request holds the slot.
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &IdempotencyRecord{
		ID:          row.ID,
		Key:         row.Key,
		RequestPath: row.RequestPath,
		RequestHash: row.RequestHash,
		Status:      row.Status,
	}, nil
}

func (r *idempotencyRepository) Finalize(ctx context.Context, key, requestPath string, statusCode int, body json.RawMessage) error {
	_, err := r.queries.FinalizeIdempotencyKey(ctx, store.FinalizeIdempotencyKeyParams{
		Key:          key,
		RequestPath:  requestPath,
		ResponseCode: int32(statusCode),
		ResponseBody: body,
	})
	return err
}
