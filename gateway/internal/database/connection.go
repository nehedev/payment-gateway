// Package database provides database connection and management utilities.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type PoolConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// NewPool opens a connection pool to Postgres and confirms it with a ping.
func NewPool(ctx context.Context, pc PoolConfig) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(pc.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	cfg.MaxConns = pc.MaxConns
	cfg.MinConns = pc.MinConns
	cfg.MaxConnLifetime = pc.MaxConnLifetime
	cfg.MaxConnIdleTime = pc.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return pool, nil
}

// Querier defines the interface for executing database queries
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const slowQueryThreshold = 200 * time.Millisecond

// loggingQuerier wraps any Querier (pool or tx) and logs duration/errors for every call
type loggingQuerier struct {
	inner  Querier
	logger *zap.Logger
}

func (l *loggingQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	start := time.Now()
	tag, err := l.inner.Exec(ctx, sql, args...)
	l.log(ctx, "exec", sql, time.Since(start), err)
	return tag, err
}

func (l *loggingQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	start := time.Now()
	rows, err := l.inner.Query(ctx, sql, args...)
	l.log(ctx, "query", sql, time.Since(start), err)
	return rows, err
}

// QueryRow can't observe the "no rows" / scan error here — pgx defers that
// until .Scan() is called on the returned Row — so only duration is logged.
func (l *loggingQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	start := time.Now()
	row := l.inner.QueryRow(ctx, sql, args...)
	l.log(ctx, "query_row", sql, time.Since(start), nil)
	return row
}

func (l *loggingQuerier) log(_ context.Context, kind, sql string, dur time.Duration, err error) {
	fields := []zap.Field{zap.String("kind", kind), zap.Duration("duration", dur)}
	switch {
	case err != nil:
		l.logger.Error("query failed", append(fields, zap.Error(err), zap.String("sql", sql))...)
	case dur > slowQueryThreshold:
		l.logger.Warn("slow query", append(fields, zap.String("sql", sql))...)
	default:
		l.logger.Debug("query executed", fields...)
	}
}

// DB wraps the connection pool and provides logged queries and transactions.
type DB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func New(pool *pgxpool.Pool, logger *zap.Logger) *DB {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DB{pool: pool, logger: logger}
}

// Ping checks the database is reachable. Used by readiness checks.
func (d *DB) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Querier returns a pool-bound Querier with logging attached. Pass this
// into sqlc's generated constructor for non-transactional queries:
//
//	queries := gen.New(db.Querier())
func (d *DB) Querier() Querier {
	return &loggingQuerier{inner: d.pool, logger: d.logger}
}

func (d *DB) Pool() *pgxpool.Pool { return d.pool }

func (d *DB) Close() { d.pool.Close() }

// TxFn is executed inside a transaction. Returning an error rolls back
type TxFn func(ctx context.Context, q Querier) error

// WithTx runs fn inside a transaction at the default (READ COMMITTED)
// isolation level. Commits on success; rolls back on error or panic
// (re-panicking after cleanup so the panic isn't silently swallowed).
func (d *DB) WithTx(ctx context.Context, fn TxFn) error {
	return d.WithTxOptions(ctx, pgx.TxOptions{}, fn)
}

func (d *DB) WithTxOptions(ctx context.Context, opts pgx.TxOptions, fn TxFn) (err error) {
	tx, err := d.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				d.logger.Error("transaction rollback failed",
					zap.Error(rbErr), zap.NamedError("original_error", err))
			}
			return
		}
		if cErr := tx.Commit(ctx); cErr != nil {
			err = fmt.Errorf("commit tx: %w", cErr)
		}
	}()

	q := &loggingQuerier{inner: tx, logger: d.logger}
	err = fn(ctx, q)
	return err
}

// WithSerializableTx runs fn in a SERIALIZABLE transaction, retrying
// automatically on Postgres serialization conflicts (error code 40001)
func (d *DB) WithSerializableTx(ctx context.Context, fn TxFn) error {
	const maxRetries = 3

	var err error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = d.WithTxOptions(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable}, fn)
		if err == nil {
			return nil
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "40001" {
			d.logger.Warn("serialization conflict, retrying",
				zap.Int("attempt", attempt), zap.Error(err))
			continue
		}
		return err
	}

	return fmt.Errorf("transaction failed after %d retries: %w", maxRetries, err)
}
