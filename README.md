# H2O GPS Tracking Platform

Enterprise-grade real-time GPS fleet tracking platform built as a **modular monolith**. Supports 100,000+ concurrent GPS devices using the GT06 binary protocol over TCP.

## Key Highlights

- **Single binary** — TCP gateway + REST API + WebSocket + workers in one process
- **GT06 protocol** — decodes Login, GPS, Heartbeat, and Alarm packets
- **100K+ devices** — goroutine-per-connection, COPY bulk inserts, Redis live cache
- **Multi-tenant** — PostgreSQL Row Level Security per tenant
- **Real-time** — sub-100ms live position via Redis; WebSocket push to browsers + mobile
- **Scale out** — add replicas; Redis keeps all instances in sync

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full design doc.

```
h2o-server (single binary, single process)
├── TCP Gateway     :8080  ← GT06 GPS devices connect here
│   └── GT06 Decoder + Connection FSM
├── In-process Event Bus (typed channels, zero serialization)
│   ├── StorageWorker  → TimescaleDB COPY (500 rows / 100ms)
│   ├── BroadcastWorker → WebSocket hub → Redis Pub/Sub fanout
│   └── AlertWorker    → rules engine (overspeed, geofence, etc.)
└── REST + WebSocket API  :8081
    └── /api/v1/* + /ws
```

## Quick Start (Docker Compose)

**Prerequisites:** Docker Desktop 4.x, 8 GB RAM

```bash
git clone https://github.com/your-org/h2o-gps-platform.git
cd h2o-gps-platform
docker compose up -d

# Follow application logs
docker compose logs -f h2o
```

| Service | URL |
|---------|-----|
| REST API | http://localhost:8081 |
| WebSocket | ws://localhost:8081/ws |
| Web Dashboard | http://localhost:3001 |
| Grafana | http://localhost:3000 (admin/admin) |
| Prometheus | http://localhost:9090 |
| GT06 TCP | tcp://localhost:8080 |

**Default credentials** (from `scripts/seed.sql`):
- Email: `admin@demo.com`
- Password: `Admin@1234`

## Build

```bash
# Build the binary
go build -o h2o-server ./cmd/server

# Build Docker image
docker build -t h2o-server:latest .

# Run locally (needs Postgres + Redis running)
POSTGRES_DSN="postgresql://h2o:h2o@localhost:5432/h2o_gps?sslmode=disable" \
REDIS_ADDRS="localhost:6379" \
JWT_ACCESS_SECRET="dev-secret-32-chars-minimum" \
JWT_REFRESH_SECRET="dev-refresh-32-chars-minimum" \
./h2o-server
```

## Kubernetes Deployment

```bash
# Deploy with base configuration (3 replicas, HPA 3–20)
kubectl apply -k deployments/k8s/base

# Deploy production overlay (5 replicas, HPA 5–50, pinned image tag)
# Edit deployments/k8s/overlays/production/kustomization.yaml first:
#   images[0].newName = your-registry/h2o-server
#   images[0].newTag  = 1.0.0
kubectl apply -k deployments/k8s/overlays/production

# Manual scale
kubectl scale deployment h2o -n h2o --replicas=10

# Watch HPA in action
kubectl get hpa h2o-hpa -n h2o -w
```

## Configuration

All configuration via environment variables (overrides `configs/config.yaml`).

| Variable | Default | Description |
|----------|---------|-------------|
| `TCP_PORT` | `8080` | GT06 TCP listen port |
| `SERVER_API_PORT` | `8081` | REST + WebSocket port |
| `TCP_MAX_CONNECTIONS` | `10000` | Per-instance connection cap |
| `POSTGRES_DSN` | — | PostgreSQL connection string |
| `REDIS_ADDRS` | — | Comma-separated Redis addresses |
| `JWT_ACCESS_SECRET` | — | **Required** — sign access tokens |
| `JWT_REFRESH_SECRET` | — | **Required** — sign refresh tokens |
| `JWT_ACCESS_TTL` | `15m` | Access token lifetime |
| `JWT_REFRESH_TTL` | `168h` | Refresh token lifetime (7 days) |
| `APP_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `APP_ENVIRONMENT` | `development` | `development` / `production` |

## API Reference

### Auth

```http
POST /api/v1/auth/login
{"email":"admin@demo.com","password":"Admin@1234"}

POST /api/v1/auth/refresh
{"refresh_token":"<token>"}

POST /api/v1/auth/logout
Authorization: Bearer <access_token>
```

### Fleet

```http
GET /api/v1/fleet/live          # batch live positions from Redis
GET /api/v1/fleet/heatmap       # aggregated heatmap from TimescaleDB
```

### Devices

```http
GET    /api/v1/devices
POST   /api/v1/devices
GET    /api/v1/devices/:id
PUT    /api/v1/devices/:id
DELETE /api/v1/devices/:id

GET    /api/v1/devices/:id/location/live
GET    /api/v1/devices/:id/location/history?from=<rfc3339>&to=<rfc3339>&limit=1000
```

### WebSocket

Connect: `ws://host:8081/ws?token=<access_token>`

Subscribe to device rooms:
```json
{"action": "subscribe", "devices": ["IMEI1", "IMEI2"]}
```

Incoming push messages:
```json
{
  "type": "location",
  "device_id": "IMEI1",
  "ts": 1718000000,
  "data": {
    "latitude": 12.9716,
    "longitude": 77.5946,
    "speed": 45.2,
    "heading": 180,
    "ignition": true
  }
}
```

## GT06 Protocol Support

| Protocol Number | Packet Type | Status |
|-----------------|-------------|--------|
| `0x01` | Login (IMEI registration) | Supported |
| `0x10` | GPS Location | Supported |
| `0x13` | Heartbeat | Supported |
| `0x16` | Alarm | Supported |
| `0x8001` | Server Response (ACK) | Supported |

Packet format: `0x7878`/`0x7979` start → length → protocol number → payload → serial number → CRC-ITU → `0x0D 0x0A` stop.

## Project Structure

```
├── cmd/server/              # Single entry point
├── internal/
│   ├── app/                 # DI root + errgroup lifecycle
│   ├── config/              # Config loading (viper)
│   ├── events/              # In-process typed pub-sub (Go generics)
│   ├── gateway/             # GT06 TCP server + decoder
│   ├── worker/              # Storage / Broadcast / Alert goroutines
│   ├── api/                 # Gin HTTP + WebSocket
│   ├── websocket/           # WS hub + Redis cross-instance fanout
│   └── storage/
│       ├── postgres/        # pgx pool, COPY bulk insert, schema
│       └── redis/           # Live cache, sessions, deduplication
├── frontend/
│   ├── web/                 # Next.js 15 + Mapbox GL + Redux Toolkit
│   └── mobile/              # React Native
├── deployments/
│   ├── docker/              # prometheus.yml
│   └── k8s/
│       ├── base/            # Namespace, Deployment, Services, HPA, Ingress
│       └── overlays/
│           └── production/  # Production image + replica overrides
├── scripts/seed.sql         # Demo tenant + 3 vehicles
├── Dockerfile               # golang:1.23-alpine → distroless/static
└── docker-compose.yml       # Full local dev stack
```

## Technology Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.23 |
| GPS Protocol | GT06 binary (custom decoder) |
| HTTP Framework | Gin |
| Database | PostgreSQL 16 + TimescaleDB |
| Cache / Pub-Sub | Redis 7 |
| Frontend Web | Next.js 15, React 19, Mapbox GL, Redux Toolkit |
| Mobile | React Native |
| Observability | Prometheus + Grafana |
| Containers | Docker + Kubernetes (Kustomize) |
| Auth | JWT (access + refresh), bcrypt |
