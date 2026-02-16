# Stage 1: Build
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /twitch-miner ./cmd/miner

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12

COPY --from=builder /twitch-miner /twitch-miner
# Only example configs are copied; real configs should be mounted via volume or created at runtime
COPY --from=builder /app/configs /configs

EXPOSE 8080

ENTRYPOINT ["/twitch-miner"]
CMD ["-config", "/configs", "-port", "8080"]
