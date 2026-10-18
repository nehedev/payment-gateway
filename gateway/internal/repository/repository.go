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
}

// IdempotencyRepository defines the contract used by the idempotency middleware.
type IdempotencyRepository interface {
	// Get returns the cached record for (key, path), or nil if none exists.
	Get(ctx context.Context, key, requestPath string) (*IdempotencyRecord, error)
	// Store persists a new record. Concurrent duplicate inserts are safe
	// (enforced by the UNIQUE constraint at the DB level).
	Store(ctx context.Context, record *IdempotencyRecord) error
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
	return &IdempotencyRecord{
		ID:           row.ID,
		Key:          row.Key,
		RequestPath:  row.RequestPath,
		RequestHash:  row.RequestHash,
		ResponseCode: int(row.ResponseCode),
		ResponseBody: row.ResponseBody,
	}, nil
}

func (r *idempotencyRepository) Store(ctx context.Context, record *IdempotencyRecord) error {
	_, err := r.queries.InsertIdempotencyKey(ctx, store.InsertIdempotencyKeyParams{
		Key:          record.Key,
		RequestPath:  record.RequestPath,
		RequestHash:  record.RequestHash,
		ResponseCode: int32(record.ResponseCode),
		ResponseBody: record.ResponseBody,
	})
	// pgx surfaces a unique-constraint violation as a *pgconn.PgError with
	// code "23505". Treat it as a no-op: the record already exists, which is
	// the desired final state (ON CONFLICT DO NOTHING semantics).
	if err != nil {
		if isUniqueViolation(err) {
			return nil
		}
		return err
	}
	return nil
}
