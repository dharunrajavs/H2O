# ─── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Cache dependencies first (layer invalidates only when go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w -X main.Version=$(git describe --tags --always 2>/dev/null || echo dev)" \
      -o /bin/h2o-server \
      ./cmd/server

# ─── Stage 2: Minimal runtime ─────────────────────────────────────────────────
# distroless/static has no shell — smallest possible attack surface
FROM gcr.io/distroless/static-debian12

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /bin/h2o-server /h2o-server

# TCP gateway port  (GPS devices)
EXPOSE 8080
# REST + WebSocket API port
EXPOSE 8081

ENTRYPOINT ["/h2o-server"]
