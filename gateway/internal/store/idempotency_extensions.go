package store

import "context"

const releaseIdempotencyKey = `
DELETE FROM idempotency_keys
WHERE key = $1 AND request_path = $2 AND status = 'in_progress'
`

func (q *Queries) ReleaseIdempotencyKey(ctx context.Context, key, requestPath string) error {
	_, err := q.db.Exec(ctx, releaseIdempotencyKey, key, requestPath)
	return err
}
