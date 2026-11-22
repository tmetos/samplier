.PHONY: help build test cover cover-check lint fmt run docker up down clean mcp demo demo-data demo-reset

DATABASE_URL ?= postgres://samplier:samplier@localhost:5432/samplier
COVERAGE_MIN ?= 80

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

build: ## Build the server binary
	go build -trimpath -o bin/samplier ./cmd/server

test: ## Run all tests
	go test ./... -count=1

cover: ## Run tests with a coverage profile
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

cover-check: cover ## Fail if coverage is below COVERAGE_MIN
	@total=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "coverage: $$total% (minimum $(COVERAGE_MIN)%)"; \
	awk -v t="$$total" -v m="$(COVERAGE_MIN)" 'BEGIN { exit !(t+0 >= m+0) }' || \
	  (echo "coverage below threshold"; exit 1)

lint: ## Check formatting and vet
	@gofmt -l ./cmd ./internal | tee /dev/stderr | (! grep .) || \
	  (echo "files need gofmt"; exit 1)
	go vet ./...

fmt: ## Format all Go sources
	gofmt -w ./cmd ./internal

run: ## Run the API locally
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/server serve

mcp: ## Run the MCP server on stdio
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/server mcp

up: ## Start PostGIS + Keycloak (applies schema.sql and seed.sql)
	docker compose up -d || docker-compose up -d

down: ## Stop the local stack
	docker compose down || docker-compose down

psql: ## Open a psql shell
	docker exec -it samplier-db-1 psql -U samplier -d samplier

demo: ## Start the full local demo (stack + data + API + UI)
	./demo/run.sh

demo-data: ## Load the demo dataset into the running database
	docker exec -i samplier-db-1 psql -U samplier -d samplier -v ON_ERROR_STOP=1 < demo/demo.sql

demo-reset: ## Recreate the demo dataset and restart the API
	./demo/run.sh --reset --api-only

demo-api: ## Start the demo API only
	./demo/run.sh --api-only

clean: ## Remove build artifacts
	rm -rf bin coverage.out dist
