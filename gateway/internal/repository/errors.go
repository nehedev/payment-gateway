package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const pgErrUniqueViolation = "23505"

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgErrUniqueViolation
}
