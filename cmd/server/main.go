// H2O GPS Tracking Platform — monolith entry point.
//
// One binary. One process. Every module (TCP gateway, REST API, WebSocket hub,
// storage worker, broadcast worker, alert engine) starts here and shares memory.
//
// To scale: run multiple instances of this binary behind a load balancer.
// Redis keeps live positions and WebSocket pub-sub in sync across instances.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/h2o/gps-platform/internal/app"
	"github.com/h2o/gps-platform/internal/config"
	"github.com/h2o/gps-platform/pkg/logger"
	"go.uber.org/zap"
)

var Version = "dev" // injected by -ldflags at build time

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	l, err := logger.New(cfg.App.LogLevel, cfg.App.Environment)
	if err != nil {
		log.Fatalf("logger: %v", err)
	}
	defer l.Sync()

	l.Info("H2O GPS Platform starting",
		zap.String("version", Version),
		zap.String("env", cfg.App.Environment),
	)

	// Root context — cancelled on SIGTERM / SIGINT
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Build the application (wires all modules together)
	application, err := app.New(ctx, cfg, l)
	if err != nil {
		l.Fatal("app init failed", zap.Error(err))
	}
	defer application.Close()

	// Run all modules — blocks until ctx is cancelled or a module errors out
	if err := application.Run(ctx); err != nil {
		l.Error("app stopped with error", zap.Error(err))
		os.Exit(1)
	}

	l.Info("H2O GPS Platform stopped cleanly")
}
