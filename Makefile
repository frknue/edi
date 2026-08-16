# edi — Life RPG. Self-hosted dev/build commands.
# Storage is PostgreSQL; local dev expects a server on localhost:5432
# (`make db-setup` creates the dev + test databases once).
DEV_DB_URL ?= postgres://localhost:5432/edi_dev?sslmode=disable

.PHONY: install dev backend frontend build prod test reset db-setup cli mcp help backup-prod deploy

help:
	@echo "edi — Life RPG"
	@echo ""
	@echo "  make install    Install Go + npm dependencies"
	@echo "  make dev        Run backend (:8080) + frontend (:5173) together"
	@echo "  make backend    Run only the Go API server (:8080)"
	@echo "  make frontend   Run only the Vite dev server (:5173)"
	@echo "  make build      Build the web client + all Go binaries -> bin/ (server, cli, mcp)"
	@echo "  make prod       Build everything and run the single self-hosted binary (:8080)"
	@echo "  make cli        Run the CLI         (e.g. make cli ARGS=dashboard)"
	@echo "  make mcp        Run the MCP server  (stdio; for AI agent clients)"
	@echo "  make test       Run backend Go tests"
	@echo "  make reset      Drop + recreate the local dev database (re-seeds on next start)"
	@echo "  make db-setup   Create the local edi_dev + edi_test Postgres databases"
	@echo ""
	@echo "  Live (Railway — push to main deploys):"
	@echo "  make backup-prod  Download the live DB into backups/ (do this before migrations)"
	@echo "  make deploy       Deploy the working tree out of band"

install:
	cd server && go mod download
	cd client && npm install

dev:
	./scripts/dev.sh

backend:
	cd server && EDI_DATABASE_URL="$(DEV_DB_URL)" go run .

frontend:
	cd client && npm run dev

build:
	cd client && npm run build
	cd server && go build -o ../bin/edi . \
		&& go build -o ../bin/edi-cli ./cmd/edi-cli \
		&& go build -o ../bin/edi-mcp ./cmd/edi-mcp

prod: build
	EDI_DATABASE_URL="$(DEV_DB_URL)" EDI_CLIENT_DIR=client/dist ./bin/edi

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
	dropdb --if-exists edi_dev && createdb edi_dev
	@echo "database reset — demo data re-seeds on next start"

# One-time local Postgres setup (dev + test databases).
db-setup:
	createdb edi_dev 2>/dev/null || true
	createdb edi_test 2>/dev/null || true
	@echo "edi_dev + edi_test ready"

# --- live deployment (Railway) ----------------------------------------------
# Push to main deploys; these are for looking after the live data.

# Dump the live database (pg_dump inside the Postgres container, over the
# Railway SSH tunnel). Postgres on Railway is the ONLY copy of the real
# characters — take one before any migration or storage change.
# Restore with: railway connect Postgres < backups/<file>.sql
backup-prod:
	@mkdir -p backups
	railway ssh --service Postgres -- sh -c 'pg_dump -U "$$PGUSER" "$$PGDATABASE"' > backups/edi-$$(date +%Y%m%d-%H%M%S).sql
	@ls -lh backups | tail -1

# Deploy the working tree out of band (normally: just push to main).
deploy:
	railway up --ci
