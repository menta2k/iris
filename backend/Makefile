.PHONY: help init plugin cli dep test cover vet lint api openapi ts wire ent gen build run docker-up docker-down

CURRENT_DIR	:= $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

help:
	@echo "Targets:"
	@echo "  init       - install protoc plugins + CLI tools"
	@echo "  dep        - go mod download"
	@echo "  api        - generate proto Go code (buf)"
	@echo "  ts         - generate proto TypeScript clients (buf)"
	@echo "  ent        - run ent codegen"
	@echo "  wire       - run wire codegen"
	@echo "  gen        - run all codegen (ent, wire, api, ts)"
	@echo "  build      - build admin service binary"
	@echo "  test       - go test ./..."
	@echo "  cover      - go test with coverage"
	@echo "  vet        - go vet"
	@echo "  lint       - golangci-lint run"
	@echo "  run        - run admin service"
	@echo "  docker-up  - start Postgres/TimescaleDB + Redis"
	@echo "  docker-down- stop docker stack"

init: plugin cli

plugin:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-errors/v2@latest
	go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest
	go install github.com/envoyproxy/protoc-gen-validate@latest
	go install github.com/go-kratos/protoc-gen-typescript-http@latest

cli:
	go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install entgo.io/ent/cmd/ent@latest
	go install github.com/google/wire/cmd/wire@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

dep:
	go mod download

test:
	go test -race -count=1 ./...

cover:
	go test -race -count=1 -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out

vet:
	go vet ./...

lint:
	golangci-lint run

# Resolve buf module deps (writes buf.lock). Idempotent; safe to re-run.
buf-deps:
	cd api && buf dep update

api: buf-deps
	cd api && buf generate

openapi: buf-deps
	cd api && buf generate --template buf.openapi.gen.yaml

ts: buf-deps
	cd api && buf generate --template buf.ts.gen.yaml

ent:
	cd app/admin/service/internal/data/ent && go run -mod=mod entgo.io/ent/cmd/ent generate ./schema

wire:
	cd app/admin/service/cmd/server && wire

# Run all codegen. `buf-deps` is run first via `api`/`ts` prerequisites.
gen: ent api ts wire

build:
	mkdir -p bin
	go build -trimpath -ldflags "-s -w" -o bin/admin-service ./app/admin/service/cmd/server

run:
	go run ./app/admin/service/cmd/server --conf ./app/admin/service/configs

docker-up:
	docker compose -f ../deploy/docker-compose.yaml up -d

docker-down:
	docker compose -f ../deploy/docker-compose.yaml down
