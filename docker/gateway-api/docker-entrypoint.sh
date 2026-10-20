#!/bin/sh
set -e

echo "Waiting for PostgreSQL to be ready..."

until nc -z -w30 "${DB_HOST:-postgres}" "${DB_PORT:-5432}"; do
  echo "Waiting for database connection..."
  sleep 2
done

echo "PostgreSQL is ready!"
echo "Running gateway database migrations..."

migrate -path /app/db/migrations \
  -database "postgres://${DB_USER:-postgres}:${DB_PASSWORD:-postgres}@${DB_HOST:-postgres}:${DB_PORT:-5432}/${DB_NAME:-mockbank}?sslmode=${DB_SSLMODE:-disable}" \
  up

echo "Gateway migrations completed successfully!"
exec "$@"
