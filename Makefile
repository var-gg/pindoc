# Pindoc dev helpers.
#
# Works in git-bash on Windows if GNU Make is installed (`winget install
# GnuWin32.Make` or similar). Users without make can run the commands
# verbatim — this file is short on purpose.

.PHONY: help db-up db-down db-logs server-build server-run server-dev web-dev fmt tidy

help:
	@echo "Pindoc dev targets (M1):"
	@echo "  db-up        — start Postgres + pgvector in Docker"
	@echo "  db-down      — stop Postgres container (data persists)"
	@echo "  db-logs      — follow Postgres logs"
	@echo "  server-build — compile the MCP server binary"
	@echo "  server-run   — run the MCP server (reads stdio)"
	@echo "  server-dev   — same as server-run, with verbose logging"
	@echo "  web-dev      — run the Vite dev server (port 5830)"
	@echo "  fmt          — gofmt + go vet the whole module"
	@echo "  tidy         — go mod tidy"

db-up:
	docker compose up -d db

db-down:
	docker compose down

db-logs:
	docker compose logs -f db

server-build:
	go build -o bin/pindoc-server$(shell go env GOEXE) ./cmd/pindoc-server

server-run:
	go run ./cmd/pindoc-server

server-dev:
	PINDOC_LOG_LEVEL=debug go run ./cmd/pindoc-server

web-dev:
	cd web && pnpm dev

fmt:
	go fmt ./...
	go vet ./...

tidy:
	go mod tidy
