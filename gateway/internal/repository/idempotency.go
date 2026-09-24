package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/nehemiah-dev/payment-gateway/internal/database"
)

// IdempotencyRecord represents a stored idempotency key with its cached response
type IdempotencyRecord struct {
	ID           int64
	Key          string
	RequestPath  string
	RequestHash  string
	ResponseCode int
	ResponseBody json.RawMessage
	CreatedAt    string
}

// IdempotencyRepository handles idempotency key storage and retrieval
type IdempotencyRepository interface {
	Get(ctx context.Context, key, requestPath string) (*IdempotencyRecord, error)
	Store(ctx context.Context, record *IdempotencyRecord) error
}

type idempotencyRepository struct {
	db *database.DB
}

// NewIdempotencyRepository creates a new idempotency repository
func NewIdempotencyRepository(db *database.DB) IdempotencyRepository {
	return &idempotencyRepository{db: db}
}

// Get retrieves a cached idempotency key and its response
func (r *idempotencyRepository) Get(ctx context.Context, key, requestPath string) (*IdempotencyRecord, error) {
	query := `
		SELECT id, key, request_path, request_hash, response_code, response_body, created_at
		FROM idempotency_keys
		WHERE key = $1 AND request_path = $2
	`

	var record IdempotencyRecord
	err := r.db.Querier().QueryRow(ctx, query, key, requestPath).Scan(
		&record.ID,
		&record.Key,
		&record.RequestPath,
		&record.RequestHash,
		&record.ResponseCode,
		&record.ResponseBody,
		&record.CreatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows || err == sql.ErrNoRows {
			return nil, nil // Not found is not an error - return nil record
		}
		return nil, fmt.Errorf("get idempotency key: %w", err)
	}

	return &record, nil
}

// Store saves an idempotency key with its response (idempotent - ON CONFLICT DO NOTHING)
func (r *idempotencyRepository) Store(ctx context.Context, record *IdempotencyRecord) error {
	query := `
		INSERT INTO idempotency_keys (key, request_path, request_hash, response_code, response_body, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (key, request_path) DO NOTHING
	`

	_, err := r.db.Querier().Exec(ctx, query,
		record.Key,
		record.RequestPath,
		record.RequestHash,
		record.ResponseCode,
		record.ResponseBody,
	)

	if err != nil {
		return fmt.Errorf("store idempotency key: %w", err)
	}

	return nil
}
