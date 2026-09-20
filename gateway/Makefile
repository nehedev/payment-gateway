.PHONY: fmt lint test build clean

BINARY_NAME := gateway
BIN_DIR := ./bin

lint:
	goimports -w -local github.com/nehemiah-dev/payment-gateway .
	gofumpt -extra -w .
	golangci-lint run ./...

test:
	go test -race -cover ./...

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/...

clean:
	rm -rf $(BIN_DIR)
	go clean -cache -testcache
up: fmt lint vuln test build

generate:
	cd api/cfg && go tool oapi-codegen -config server.yaml ../openapi.yaml
	cd api/cfg && go tool oapi-codegen -config spec.yaml ../openapi.yaml