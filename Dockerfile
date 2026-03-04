# =============================================================================
# Signal-Flow — Multi-stage Docker build
# =============================================================================
# Build:  docker build -t signal-flow .
# Run:    docker run --env-file .env signal-flow pipeline run
# Serve:  docker run --env-file .env -p 8080:8080 signal-flow serve
# =============================================================================

# --- Stage 1: Build ---
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

# Build the binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /signal-flow ./cmd/signal-flow

# --- Stage 2: Runtime ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /signal-flow /usr/local/bin/signal-flow

# Create config directory.
RUN mkdir -p /root/.config/signal-flow

# Default to showing help. Override with:
#   docker run signal-flow pipeline run
#   docker run signal-flow serve
ENTRYPOINT ["signal-flow"]
CMD ["--help"]
