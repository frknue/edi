# edi — Life RPG. Self-hosted dev/build commands.
DB ?= edi.db

.PHONY: install dev backend frontend build prod test reset cli mcp help

help:
	@echo "edi — Life RPG"
	@echo ""
	@echo "  make install    Install Go + npm dependencies"
	@echo "  make dev        Run backend (:8080) + frontend (:5173) together"
	@echo "  make backend    Run only the Go API server (:8080)"
	@echo "  make frontend   Run only the Vite dev server (:5173)"
	@echo "  make build      Build the web client + all Go binaries -> bin/ (server, cli, mcp, telegram)"
	@echo "  make prod       Build everything and run the single self-hosted binary (:8080)"
	@echo "  make cli        Run the CLI         (e.g. make cli ARGS=dashboard)"
	@echo "  make mcp        Run the MCP server  (stdio; for AI agent clients)"
	@echo "  make test       Run backend Go tests"
	@echo "  make reset      Delete the SQLite database (re-seeds on next start)"

install:
	cd server && go mod download
	cd client && npm install

dev:
	./scripts/dev.sh

backend:
	cd server && EDI_DB=$(DB) go run .

frontend:
	cd client && npm run dev

build:
	cd client && npm run build
	cd server && go build -o ../bin/edi . \
		&& go build -o ../bin/edi-cli ./cmd/edi-cli \
		&& go build -o ../bin/edi-mcp ./cmd/edi-mcp \
		&& go build -o ../bin/edi-telegram ./cmd/edi-telegram

prod: build
	EDI_DB=$(DB) EDI_CLIENT_DIR=client/dist ./bin/edi

# Run the CLI against a running server. Example: make cli ARGS="complete 1"
ARGS ?= dashboard
cli:
	cd server && go run ./cmd/edi-cli $(ARGS)

# Run the MCP stdio server (point your AI client at this command).
mcp:
	cd server && go run ./cmd/edi-mcp

test:
	cd server && go test ./...

reset:
	rm -f server/$(DB) server/$(DB)-shm server/$(DB)-wal $(DB) $(DB)-shm $(DB)-wal
	@echo "database reset — demo data re-seeds on next start"
