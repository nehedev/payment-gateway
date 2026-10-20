include .env
export
.PHONY: fmt lint vuln test build clean migrate-up migrate-down migrate-down-all migrate-version migrate-create sqlc-generate api-generate

BINARY_NAME := gateway
BIN_DIR := ./bin
MIGRATIONS_DIR = db/migrations
DATABASE_URL = postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

fmt:
	goimports -w -local github.com/nehemiah-dev/payment-gateway .
	gofumpt -extra -w .

lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

test:
	go test -race -cover ./...

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/...

clean:
	rm -rf $(BIN_DIR)
	go clean -cache -testcache
up: fmt lint vuln test build

migrate-up:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) down 1

migrate-down-all:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) down -all

migrate-version:
	migrate -database "$(DATABASE_URL)" -path $(MIGRATIONS_DIR) version

migrate-create:
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

sqlc-generate:
	sqlc generate -f db/sqlc.yaml

api-generate:
	oapi-codegen -config api/cfg/server.yaml api/openapi.yaml
	oapi-codegen -config api/cfg/spec.yaml api/openapi.yaml
