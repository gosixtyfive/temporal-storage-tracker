# Local development targets. See docs/architecture.md#local-development.
# Run temporal, server and frontend in separate terminals.

STORAGE_DURATION ?= 7m

.PHONY: temporal server frontend frontend-install frontend-check test build vet

temporal: ## Temporal dev server (UI on :8233)
	temporal server start-dev

server: ## Run the API + worker + store on :8080 (compressed demo timing by default)
	cd backend && STORAGE_DURATION=$(STORAGE_DURATION) go run ./cmd/server

frontend: ## Nuxt dev server on :3000; proxies /api to the server on :8080
	cd frontend && npm run dev

frontend-install: ## Install frontend dependencies from the lockfile
	cd frontend && npm ci

frontend-check: ## Typecheck and unit-test the frontend
	cd frontend && npm run typecheck && npm test

test: ## Run backend tests
	cd backend && go test ./...

vet:
	cd backend && go vet ./...

build: ## Build backend binaries into backend/bin
	cd backend && go build -o bin/ ./cmd/...
