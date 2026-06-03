# ASCEND — Life RPG. Self-hosted dev/build commands.
DB ?= liferpg.db

.PHONY: install dev backend frontend build prod test reset help

help:
	@echo "ASCEND — Life RPG"
	@echo ""
	@echo "  make install    Install Go + npm dependencies"
	@echo "  make dev        Run backend (:8080) + frontend (:5173) together"
	@echo "  make backend    Run only the Go API server (:8080)"
	@echo "  make frontend   Run only the Vite dev server (:5173)"
	@echo "  make build      Build the web client and a single Go binary -> bin/liferpg"
	@echo "  make prod       Build everything and run the single self-hosted binary (:8080)"
	@echo "  make test       Run backend Go tests"
	@echo "  make reset      Delete the SQLite database (re-seeds on next start)"

install:
	cd server && go mod download
	cd client && npm install

dev:
	./scripts/dev.sh

backend:
	cd server && LIFERPG_DB=$(DB) go run .

frontend:
	cd client && npm run dev

build:
	cd client && npm run build
	cd server && go build -o ../bin/liferpg .

prod: build
	LIFERPG_DB=$(DB) LIFERPG_CLIENT_DIR=client/dist ./bin/liferpg

test:
	cd server && go test ./...

reset:
	rm -f server/$(DB) server/$(DB)-shm server/$(DB)-wal $(DB) $(DB)-shm $(DB)-wal
	@echo "database reset — demo data re-seeds on next start"
