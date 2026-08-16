# Multi-stage build: web client -> Go server -> minimal runtime image.
# Produces the same single self-hosted server as `make prod`, containerized.

# --- Stage 1: build the web client ---
FROM node:22-alpine AS client
WORKDIR /build/client
COPY client/package.json client/package-lock.json ./
RUN npm ci
COPY client/ ./
RUN npm run build

# --- Stage 2: build the Go server (pure Go, no CGO) ---
FROM golang:1.25-alpine AS server
WORKDIR /build/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
# Push to main deploys, so the build is a gate: vet + the pure tests run here.
# DB-backed tests skip in this stage (no Postgres in the builder — see
# internal/db/dbtest); the full suite runs locally and in GitHub Actions,
# which provides a postgres service container.
RUN go vet ./... && go test ./...
# The server, plus the Telegram companion so one image can back both Railway
# services (the bot service just overrides the start command).
RUN CGO_ENABLED=0 go build -o /build/edi . \
	&& CGO_ENABLED=0 go build -o /build/edi-telegram ./cmd/edi-telegram

# --- Stage 3: runtime ---
# ca-certificates: HTTPS to the OpenAI endpoints. tzdata: local-day math
# (decay, journal daily XP) honors the TZ env var instead of falling back to UTC.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=server /build/edi ./edi
COPY --from=server /build/edi-telegram ./edi-telegram
COPY --from=client /build/client/dist ./client/dist
ENV EDI_CLIENT_DIR=/app/client/dist
EXPOSE 8080
# Storage is PostgreSQL: set DATABASE_URL (Railway injects it when the
# Postgres service is referenced) or EDI_DATABASE_URL.
CMD ["/app/edi"]
