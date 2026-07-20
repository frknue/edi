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
FROM golang:1.24-alpine AS server
WORKDIR /build/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -o /build/edi .

# --- Stage 3: runtime ---
# ca-certificates: HTTPS to the OpenAI endpoints. tzdata: local-day math
# (decay, journal daily XP) honors the TZ env var instead of falling back to UTC.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && mkdir -p /data
WORKDIR /app
COPY --from=server /build/edi ./edi
COPY --from=client /build/client/dist ./client/dist
ENV EDI_CLIENT_DIR=/app/client/dist
EXPOSE 8080
# DB path resolution: explicit EDI_DB wins, else the Railway volume mount if one
# is attached, else /data (baked into the image; ephemeral without a volume).
CMD ["/bin/sh", "-c", "EDI_DB=\"${EDI_DB:-${RAILWAY_VOLUME_MOUNT_PATH:-/data}/edi.db}\" exec /app/edi"]
