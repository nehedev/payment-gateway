package repository

import (
	"context"

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

func (r *Repository) CreatePayment(ctx context.Context, arg store.CreatePaymentParams) (store.Payment, error) {
	return r.queries.CreatePayment(ctx, arg)
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
