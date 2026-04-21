# Pindoc dev helpers.
#
# Works in git-bash on Windows if GNU Make is installed (`winget install
# GnuWin32.Make` or similar). Users without make can run the commands
# verbatim — this file is short on purpose.

.PHONY: help db-up db-down db-logs embed-up embed-down embed-logs server-build api-build reembed-build server-run server-dev server-run-http api-run-http web-dev fmt tidy

# Embedding env used by the http provider (Phase 10). Keep the model hint,
# dimension, and E5-style prefixes together so a single `make server-run-http`
# brings the MCP server up with the real embedder wired in.
EMBED_ENV := \
  PINDOC_EMBED_PROVIDER=http \
  PINDOC_EMBED_ENDPOINT=http://127.0.0.1:5860/v1/embeddings \
  PINDOC_EMBED_MODEL=multilingual-e5-base \
  PINDOC_EMBED_DIM=768 \
  PINDOC_EMBED_MAX_TOKENS=512 \
  PINDOC_EMBED_PREFIX_QUERY="query: " \
  PINDOC_EMBED_PREFIX_DOCUMENT="passage: "

help:
	@echo "Pindoc dev targets (M1):"
	@echo "  db-up            — start Postgres + pgvector in Docker"
	@echo "  db-down          — stop Postgres container (data persists)"
	@echo "  db-logs          — follow Postgres logs"
	@echo "  embed-up         — start TEI embedding server (port 5860)"
	@echo "  embed-down       — stop TEI container (cache persists)"
	@echo "  embed-logs       — follow TEI logs"
	@echo "  server-build     — compile the MCP server binary"
	@echo "  api-build        — compile the HTTP API binary"
	@echo "  reembed-build    — compile the pindoc-reembed CLI"
	@echo "  server-run       — run the MCP server with stub embedder"
	@echo "  server-run-http  — run the MCP server with TEI http embedder"
	@echo "  api-run-http     — run the HTTP API with TEI http embedder"
	@echo "  web-dev          — run the Vite dev server (port 5830)"
	@echo "  fmt              — gofmt + go vet the whole module"
	@echo "  tidy             — go mod tidy"

db-up:
	docker compose up -d db

db-down:
	docker compose down

db-logs:
	docker compose logs -f db

embed-up:
	docker compose up -d embed

embed-down:
	docker compose stop embed

embed-logs:
	docker compose logs -f embed

server-build:
	go build -o bin/pindoc-server$(shell go env GOEXE) ./cmd/pindoc-server

api-build:
	go build -o bin/pindoc-api$(shell go env GOEXE) ./cmd/pindoc-api

reembed-build:
	go build -o bin/pindoc-reembed$(shell go env GOEXE) ./cmd/pindoc-reembed

server-run:
	go run ./cmd/pindoc-server

server-dev:
	PINDOC_LOG_LEVEL=debug go run ./cmd/pindoc-server

server-run-http:
	$(EMBED_ENV) go run ./cmd/pindoc-server

api-run-http:
	$(EMBED_ENV) go run ./cmd/pindoc-api

web-dev:
	cd web && pnpm dev

fmt:
	go fmt ./...
	go vet ./...

tidy:
	go mod tidy
