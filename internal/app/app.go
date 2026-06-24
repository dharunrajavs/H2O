// Package app wires every module of the monolith together and manages their
// shared lifecycle. Think of it as the dependency-injection root.
//
// Startup order:
//   1. Infrastructure (Postgres, Redis)
//   2. In-process event bus
//   3. WebSocket hub (needs Redis for cross-instance fanout)
//   4. TCP gateway (needs event bus + cache)
//   5. Background workers (need event bus + DB)
//   6. HTTP/WS API server (needs DB + cache + hub)
//
// All components share the same context; cancelling it shuts the whole monolith
// down gracefully in reverse order via errgroup.
package app

import (
	"context"
	"fmt"

	"github.com/h2o/gps-platform/internal/api"
	"github.com/h2o/gps-platform/internal/api/handlers"
	"github.com/h2o/gps-platform/internal/config"
	"github.com/h2o/gps-platform/internal/events"
	"github.com/h2o/gps-platform/internal/gateway"
	"github.com/h2o/gps-platform/internal/storage/postgres"
	redisstore "github.com/h2o/gps-platform/internal/storage/redis"
	"github.com/h2o/gps-platform/internal/worker"
	internws "github.com/h2o/gps-platform/internal/websocket"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// App is the monolith. It owns all modules and their lifecycle.
type App struct {
	cfg *config.Config
	log *zap.Logger

	// Infrastructure
	db    *postgres.DB
	redis goredis.UniversalClient
	cache *redisstore.Cache

	// In-process event bus — zero-copy channel fan-out
	bus *events.Bus

	// Modules
	tcpServer       *gateway.Server
	wsHub           *internws.Hub
	apiServer       *api.Server
	storageWorker   *worker.StorageWorker
	broadcastWorker *worker.BroadcastWorker
	alertWorker     *worker.AlertWorker
}

// New builds and wires all modules. It does not start any goroutines.
func New(ctx context.Context, cfg *config.Config, log *zap.Logger) (*App, error) {
	a := &App{cfg: cfg, log: log}

	// ── 1. Infrastructure ────────────────────────────────────────────────────

	var err error
	a.db, err = postgres.New(ctx, &cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}

	a.redis, err = redisstore.NewClient(&cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("redis: %w", err)
	}
	a.cache = redisstore.NewCache(a.redis)

	// ── 2. In-process event bus (replaces Redis Streams) ────────────────────

	a.bus = &events.Bus{}

	// ── 3. WebSocket hub ─────────────────────────────────────────────────────

	a.wsHub = internws.NewHub(a.redis, log)

	// ── 4. TCP gateway ───────────────────────────────────────────────────────

	resolver := gateway.NewCachedDeviceResolver(a.cache, log)
	handler  := gateway.NewHandler(resolver, a.bus, a.cache, log)
	a.tcpServer = gateway.NewServer(&cfg.TCP, handler, log)

	// ── 5. Background workers ────────────────────────────────────────────────

	a.storageWorker   = worker.NewStorageWorker(a.db, a.bus, log)
	a.broadcastWorker = worker.NewBroadcastWorker(a.wsHub, a.bus, log)
	a.alertWorker     = worker.NewAlertWorker(a.bus, log)

	// ── 6. HTTP + WebSocket API server ───────────────────────────────────────

	deps := &api.Dependencies{
		DeviceHandler:   handlers.NewDeviceHandler(a.db, a.cache, log),
		LocationHandler: handlers.NewLocationHandler(a.db, a.cache, log),
		AuthHandler: handlers.NewAuthHandler(
			a.db,
			cfg.JWT.AccessSecret, cfg.JWT.RefreshSecret,
			cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL,
			log,
		),
		FleetHandler: handlers.NewFleetHandler(a.db, a.cache, log),
		AlertHandler: handlers.NewAlertHandler(a.db, log),
		WSHandler:    handlers.NewWSHandler(a.wsHub, log),
		JWTSecret:    cfg.JWT.AccessSecret,
	}
	a.apiServer = api.NewServer(&cfg.Server, deps, log)

	return a, nil
}

// Run starts every module concurrently and blocks until all of them stop.
// The first module to return a non-nil error cancels the whole group.
func (a *App) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	// WebSocket hub — must start before API server accepts connections
	g.Go(func() error {
		a.wsHub.Run(ctx)
		return nil
	})

	// Background workers — subscribe to bus before TCP gateway starts publishing
	g.Go(func() error { return a.storageWorker.Run(ctx) })
	g.Go(func() error { return a.broadcastWorker.Run(ctx) })
	g.Go(func() error { return a.alertWorker.Run(ctx) })

	// TCP gateway — GPS devices connect here
	g.Go(func() error { return a.tcpServer.Start(ctx) })

	// HTTP + WebSocket API server
	g.Go(func() error { return a.apiServer.Start(ctx) })

	a.log.Info("H2O monolith running",
		zap.String("tcp", fmt.Sprintf(":%d", a.cfg.TCP.Port)),
		zap.String("api", fmt.Sprintf(":%d", a.cfg.Server.APIPort)))

	return g.Wait()
}

// Close releases infrastructure resources. Call after Run returns.
func (a *App) Close() {
	a.bus.Close()
	if a.db != nil {
		a.db.Close()
	}
	if a.redis != nil {
		_ = a.redis.Close()
	}
	a.log.Info("app shut down cleanly")
}
