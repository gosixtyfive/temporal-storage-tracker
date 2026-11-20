# Local development targets. See docs/architecture.md#local-development.
# Run temporal, worker, api (and later frontend) in separate terminals.

STORAGE_DURATION ?= 7m

.PHONY: temporal worker api test build vet

temporal: ## Temporal dev server with the ItemStatus search attribute (UI on :8233)
	temporal server start-dev --search-attribute ItemStatus=Keyword

worker: ## Run the Temporal worker
	cd backend && go run ./cmd/worker

api: ## Run the HTTP API (compressed demo timing by default)
	cd backend && STORAGE_DURATION=$(STORAGE_DURATION) go run ./cmd/api

test: ## Run backend tests
	cd backend && go test ./...

vet:
	cd backend && go vet ./...

build: ## Build backend binaries into backend/bin
	cd backend && go build -o bin/ ./cmd/...
