# Iris backend developer tasks.
GOBIN ?= $(shell go env GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

.PHONY: tools
tools: ## Install code-generation and lint tooling
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
	go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest

.PHONY: generate
generate: ## Generate proto, gRPC, HTTP, and OpenAPI artifacts
	buf dep update
	buf generate

.PHONY: build
build: ## Build the iris binary
	go build -o bin/iris ./cmd/iris

.PHONY: build-embed
build-embed: ## Build the frontend and a backend binary with the SPA embedded (production)
	cd ../frontend && npm ci && npm run build
	rm -rf internal/webui/dist && cp -r ../frontend/dist internal/webui/dist
	go build -trimpath -tags embed_ui -ldflags="-s -w" -o bin/iris ./cmd/iris

.PHONY: fmt
fmt: ## Format Go sources
	gofmt -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: test
test: ## Run unit and integration tests (serial packages: DB-backed tests share one database)
	go test -p 1 ./...

.PHONY: race
race: ## Run tests with the race detector
	go test -race -p 1 ./...

.PHONY: test-e2e
test-e2e: ## Run live KumoMTA e2e tests (opt-in; needs docker + IRIS_E2E=1)
	IRIS_E2E=1 go test -tags e2e -p 1 -v -count=1 ./tests/e2e/...

.PHONY: test-policy-load
test-policy-load: ## Validate the generated policy loads in a real kumod (needs docker; CI-safe, no DB)
	IRIS_E2E=1 go test -tags e2e -p 1 -v -count=1 -run TestGeneratedPolicyLoadsInKumod ./tests/e2e/...

.PHONY: lint
lint: ## Run staticcheck (if installed)
	staticcheck ./... || true

.PHONY: security
security: ## Run vulnerability and security scanners (best effort)
	govulncheck ./... || true
	gosec ./... || true

.PHONY: openapi-validate
openapi-validate: ## Validate the generated OpenAPI document
	@test -f api/iris/admin/v1/openapi.yaml && echo "openapi.yaml present" || (echo "missing openapi.yaml" && exit 1)

.PHONY: check
check: fmt vet test ## Run formatting, vet, and tests
