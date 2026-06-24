# H2O GPS Platform — Architecture

## Overview

H2O is a **modular monolith** that runs as a single binary (`h2o-server`).
Every subsystem — TCP gateway, REST/WebSocket API, storage writer, broadcast
router, alert engine — executes as a goroutine inside one OS process and
communicates via in-process typed channels (zero serialization overhead).

Horizontal scaling is achieved by running **N identical replicas** behind a
load balancer. Redis keeps live state (position cache, WebSocket fan-out) in
sync across all replicas.

```
┌──────────────────────────────────────────────────────────────────┐
│                     h2o-server  (single binary)                  │
│                                                                  │
│  ┌─────────────┐    ┌──────────────────────────────────────────┐ │
│  │ TCP Gateway │    │            In-Process Event Bus           │ │
│  │  port 8080  │───▶│  Topic[GPSEvent]   Topic[AlarmEvent]     │ │
│  │  GT06 proto │    │  Topic[LoginEvent] Topic[HeartbeatEvent]  │ │
│  └─────────────┘    └────────┬────────────────┬────────────────┘ │
│                              │                │                  │
│                    ┌─────────▼──────┐ ┌───────▼──────────┐      │
│                    │ StorageWorker  │ │ BroadcastWorker  │      │
│                    │ COPY→TimescaleDB│ │  WS Hub fanout  │      │
│                    └────────────────┘ └──────────────────┘      │
│                              │                                   │
│                    ┌─────────▼──────┐                           │
│                    │  AlertWorker   │                           │
│                    │ rules engine   │                           │
│                    └────────────────┘                           │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │          REST + WebSocket API   port 8081                 │   │
│  │  /api/v1/auth  /api/v1/devices  /api/v1/fleet  /ws       │   │
│  └──────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
        │                │                │
   TimescaleDB        Redis           Redis Pub/Sub
  (locations,       (live pos,     (WS cross-instance
   users, alerts)   sessions)         fanout)
```

---

## Module Breakdown

### TCP Gateway (`internal/gateway/`)

Accepts raw TCP connections from GT06-compatible GPS devices.

| Component | Responsibility |
|-----------|---------------|
| `server.go` | `net.Listen` on `:8080`, goroutine per connection, TCP_NODELAY + keepalive |
| `connection.go` | FSM: New → Active → Closed; rolling buffer accumulation; frame dispatch |
| `handler.go` | Decodes GT06 frames, publishes to event bus, writes live position to Redis |
| `resolver.go` | Resolves IMEI → (tenantID, deviceID) with Redis cache |

**GT06 packet flow:**

```
Device TCP ──► Connection.readLoop ──► decoder.DecodeFrame
                                              │
                     ┌────────────────────────┤
                     ▼                        ▼
              handler.handleLogin    handler.handleGPS
                     │                        │
              redis.SetSession    redis.SetLivePosition  ← immediate (sub-1ms)
                     │                        │
              bus.Login.Publish        bus.GPS.Publish   ← async fan-out
```

### In-Process Event Bus (`internal/events/bus.go`)

Generic typed pub-sub using Go 1.23 generics. No serialization, no network hop.

```go
type Topic[T any] struct { subs []chan T }
func (t *Topic[T]) Subscribe(bufSize int) <-chan T
func (t *Topic[T]) Publish(v T)           // non-blocking fan-out
```

`Bus` holds four topics: `GPS`, `Alarm`, `Login`, `Heartbeat`.
Publisher never blocks: if a subscriber's channel is full the event is dropped
for that subscriber (back-pressure protection for slow consumers).

### Workers (`internal/worker/`)

| Worker | Subscribes to | Does |
|--------|--------------|------|
| `StorageWorker` | `bus.GPS` (buf 8192) | Batches 500 rows → COPY to TimescaleDB every 100ms |
| `BroadcastWorker` | `bus.GPS` (buf 4096) | Marshals JSON → `hub.BroadcastToDevice` |
| `AlertWorker` | `bus.Alarm` + `bus.GPS` | Hardware alarms + rule evaluation (overspeed etc.) |

### WebSocket Hub (`internal/websocket/`)

- **Local fanout**: `hub.BroadcastToDevice` sends to all clients subscribed on this pod.
- **Cross-instance fanout**: publishes to Redis `ws:broadcast:{deviceID}`; all other pod
  instances receive via `PSubscribe("ws:broadcast:*")` and deliver locally.
- Clients send `{"action":"subscribe","devices":["imei1","imei2"]}` to join rooms.

### Storage (`internal/storage/`)

| Store | Technology | Key patterns |
|-------|-----------|-------------|
| Locations | TimescaleDB hypertable, 1-week chunks, compressed after 7 days | RLS via `app.current_tenant_id` |
| Live position | Redis Hash `device:{imei}:live` | Expires 5 min after last update |
| Sessions | Redis Hash `device:{imei}:session` | TTL tied to device keepalive |
| Deduplication | Redis SETNX `dedup:{imei}:{serial}` 60s TTL | Prevents duplicate GPS rows |
| Auth tokens | PostgreSQL `refresh_tokens` table | `revoked_at` nullable |

Multi-tenancy is enforced via **PostgreSQL Row Level Security** — every query
runs `SET LOCAL app.current_tenant_id = '<id>'` before touching data.

### API (`internal/api/`)

Gin router on `:8081`.

```
GET  /health          → liveness + readiness probe
GET  /metrics         → Prometheus exposition format
GET  /ws              → WebSocket upgrade

POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout

GET/POST/PUT/DELETE  /api/v1/devices
GET                  /api/v1/devices/:id/location/live
GET                  /api/v1/devices/:id/location/history

GET  /api/v1/fleet/live
GET  /api/v1/fleet/heatmap

GET/POST/PUT/DELETE  /api/v1/alerts
GET/POST/PUT/DELETE  /api/v1/geofences
GET                  /api/v1/reports/*
```

---

## Scaling Strategy

```
                     ┌────────────────────────┐
GPS Devices ──TCP──▶ │   NLB / LoadBalancer   │
Web / Mobile ─HTTP─▶ │   (AWS NLB / nginx)    │
                     └───────┬────────────────┘
                             │  round-robin
            ┌────────────────┼────────────────┐
            ▼                ▼                ▼
         h2o pod 1       h2o pod 2       h2o pod N
         (8080+8081)     (8080+8081)     (8080+8081)
            │                │                │
            └────────────────┼────────────────┘
                             │  shared state
                    ┌────────┴──────────┐
                    │      Redis        │  ← live positions + WS pub-sub
                    │   TimescaleDB     │  ← history + alerts
                    └───────────────────┘
```

- Each replica is **stateless** — Redis holds all shared runtime state.
- Add replicas with `kubectl scale deployment h2o --replicas=N` or let HPA do it.
- HPA triggers at 70% CPU / 80% memory; scales 3 → 50 pods automatically.
- TCP sessions are sticky to one pod for their lifetime — no stickiness needed at
  the LB level because each device maintains one persistent connection.

---

## Security

| Control | Implementation |
|---------|---------------|
| Authentication | JWT; access 15m, refresh 7d |
| Password storage | bcrypt cost 12 |
| Multi-tenancy | PostgreSQL RLS + `app.current_tenant_id` |
| Transport | TLS terminated at Ingress / NLB |
| Rate limiting | `golang.org/x/time/rate` per source IP |
| Container | `gcr.io/distroless/static-debian12` — no shell, no package manager |
| Secrets | Kubernetes Secrets (use External Secrets Operator in production) |

---

## Data Flow — End to End

```
GPS Device                 h2o-server Pod                  Redis          TimescaleDB
    │                           │                             │                │
    │──GT06 Login packet──────▶ │                             │                │
    │                      DecodeLogin                        │                │
    │                      ResolveIMEI ──────────────────────▶│                │
    │                           │◀───────────(tenantID,devID)─│                │
    │◀──Server Response (ACK)── │                             │                │
    │                           │                             │                │
    │──GT06 GPS packet────────▶ │                             │                │
    │                      DecodeGPS                          │                │
    │                      SetLivePosition ───HSET────────────▶│                │
    │                      bus.GPS.Publish                    │                │
    │◀──ACK──────────────────── │                             │                │
    │                           │                             │                │
    │               StorageWorker (batch 500)                 │                │
    │                           │──────────────COPY───────────────────────────▶│
    │                           │                             │                │
    │               BroadcastWorker                           │                │
    │                           │──BroadcastToDevice (local)  │                │
    │                           │──PUBLISH ws:broadcast:*────▶│                │
    │                           │    (other pods PSubscribe)  │                │
    │                           │                             │                │
Web Dashboard ◀──────WS push (< 50ms end-to-end)───────────────────────────────
```

---

## Application Wiring (`internal/app/app.go`)

```
app.New(ctx, cfg, log)
  │
  ├── postgres.New(ctx, &cfg.Postgres)          // pgx pool
  ├── redisstore.NewClient(&cfg.Redis)          // single-node or cluster
  ├── redisstore.NewCache(redis)
  │
  ├── &events.Bus{}                             // in-process typed channels
  │
  ├── internws.NewHub(redis, log)               // WebSocket hub
  │
  ├── gateway.NewCachedDeviceResolver(cache, log)
  ├── gateway.NewHandler(resolver, bus, cache, log)
  ├── gateway.NewServer(&cfg.TCP, handler, log) // TCP listener
  │
  ├── worker.NewStorageWorker(db, bus, log)
  ├── worker.NewBroadcastWorker(wsHub, bus, log)
  ├── worker.NewAlertWorker(bus, log)
  │
  └── api.NewServer(&cfg.Server, deps, log)    // Gin HTTP + WebSocket

app.Run(ctx)  →  errgroup starts all 6 goroutines concurrently
```

---

## Directory Structure

```
H2O/
├── cmd/server/              ← single entry point (main.go)
├── internal/
│   ├── app/                 ← DI root — wires all modules, errgroup lifecycle
│   ├── config/              ← viper config + env overrides
│   ├── events/              ← in-process typed pub-sub bus (generics)
│   ├── gateway/             ← TCP listener, GT06 decoder, connection FSM
│   ├── worker/              ← storage, broadcast, alert goroutines
│   ├── api/                 ← Gin HTTP + WebSocket server
│   ├── websocket/           ← WS hub with Redis cross-instance fanout
│   └── storage/
│       ├── postgres/        ← pgx pool, COPY bulk insert, schema.sql
│       └── redis/           ← live cache, sessions, deduplication
├── frontend/
│   ├── web/                 ← Next.js 15 + Mapbox GL + Redux Toolkit
│   └── mobile/              ← React Native
├── deployments/
│   ├── docker/              ← prometheus.yml
│   └── k8s/
│       ├── base/            ← namespace, deployment, services, HPA, ingress
│       └── overlays/production/
├── scripts/seed.sql         ← demo tenant + 3 vehicles + 3 devices
├── Dockerfile               ← golang:1.23-alpine → distroless/static
├── docker-compose.yml       ← local dev: postgres + redis + h2o + web + monitoring
└── go.mod                   ← module github.com/h2o/gps-platform, go 1.23
```
