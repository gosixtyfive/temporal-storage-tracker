# Local development targets. See docs/architecture.md#local-development.
# Run temporal, server (and later frontend) in separate terminals.

STORAGE_DURATION ?= 7m

.PHONY: temporal server test build vet

temporal: ## Temporal dev server (UI on :8233)
	temporal server start-dev

server: ## Run the API + worker + store on :8080 (compressed demo timing by default)
	cd backend && STORAGE_DURATION=$(STORAGE_DURATION) go run ./cmd/server

test: ## Run backend tests
	cd backend && go test ./...

vet:
	cd backend && go vet ./...

build: ## Build backend binaries into backend/bin
	cd backend && go build -o bin/ ./cmd/...
