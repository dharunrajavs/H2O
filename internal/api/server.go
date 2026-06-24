package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/h2o/gps-platform/internal/api/handlers"
	"github.com/h2o/gps-platform/internal/api/middleware"
	"github.com/h2o/gps-platform/internal/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server is the HTTP API server for web/mobile clients
type Server struct {
	cfg    *config.ServerConfig
	router *gin.Engine
	srv    *http.Server
	log    *zap.Logger
}

// Dependencies bundles all handler dependencies
type Dependencies struct {
	DeviceHandler   *handlers.DeviceHandler
	LocationHandler *handlers.LocationHandler
	AuthHandler     *handlers.AuthHandler
	FleetHandler    *handlers.FleetHandler
	AlertHandler    *handlers.AlertHandler
	WSHandler       *handlers.WSHandler
	JWTSecret       string
}

// NewServer creates the HTTP API server with all routes configured
func NewServer(cfg *config.ServerConfig, deps *Dependencies, log *zap.Logger) *Server {
	if cfg.APIPort == 0 {
		// production mode: suppress debug noise from gin
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	s := &Server{
		cfg:    cfg,
		router: router,
		log:    log,
	}

	s.setupMiddleware(deps)
	s.setupRoutes(deps)

	return s
}

func (s *Server) setupMiddleware(deps *Dependencies) {
	s.router.Use(
		middleware.RequestID(),
		middleware.Logger(s.log),
		middleware.Recovery(s.log),
		middleware.CORS(),
		middleware.RateLimit(100, time.Minute),
	)
}

func (s *Server) setupRoutes(deps *Dependencies) {
	r := s.router

	// Health & metrics endpoints (no auth)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now()})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// WebSocket endpoint
	r.GET("/ws", middleware.JWTAuth(deps.JWTSecret), deps.WSHandler.Handle)

	api := r.Group("/api/v1")
	{
		// ─── Auth ──────────────────────────────────────────────────────────
		auth := api.Group("/auth")
		{
			auth.POST("/login", deps.AuthHandler.Login)
			auth.POST("/refresh", deps.AuthHandler.Refresh)
			auth.POST("/logout", middleware.JWTAuth(deps.JWTSecret), deps.AuthHandler.Logout)
		}

		// ─── Protected routes ──────────────────────────────────────────────
		protected := api.Group("", middleware.JWTAuth(deps.JWTSecret))
		{
			// Devices
			devices := protected.Group("/devices")
			{
				devices.GET("", deps.DeviceHandler.List)
				devices.POST("", deps.DeviceHandler.Create)
				devices.GET("/:imei", deps.DeviceHandler.Get)
				devices.PUT("/:imei", deps.DeviceHandler.Update)
				devices.DELETE("/:imei", deps.DeviceHandler.Delete)
				devices.GET("/:imei/live", deps.LocationHandler.GetLive)
				devices.GET("/:imei/history", deps.LocationHandler.GetHistory)
				devices.GET("/:imei/trips", deps.LocationHandler.GetTrips)
			}

			// Fleet
			fleet := protected.Group("/fleet")
			{
				fleet.GET("", deps.FleetHandler.Summary)
				fleet.GET("/live", deps.FleetHandler.LiveAll)
				fleet.GET("/heatmap", deps.FleetHandler.Heatmap)
			}

			// Alerts
			alerts := protected.Group("/alerts")
			{
				alerts.GET("", deps.AlertHandler.List)
				alerts.PUT("/:id/read", deps.AlertHandler.MarkRead)
			}

			// Alert Rules
			rules := protected.Group("/alert-rules")
			{
				rules.GET("", deps.AlertHandler.ListRules)
				rules.POST("", deps.AlertHandler.CreateRule)
				rules.PUT("/:id", deps.AlertHandler.UpdateRule)
				rules.DELETE("/:id", deps.AlertHandler.DeleteRule)
			}

			// Geofences
			geo := protected.Group("/geofences")
			{
				geo.GET("", deps.FleetHandler.ListGeofences)
				geo.POST("", deps.FleetHandler.CreateGeofence)
				geo.PUT("/:id", deps.FleetHandler.UpdateGeofence)
				geo.DELETE("/:id", deps.FleetHandler.DeleteGeofence)
			}

			// Reports
			reports := protected.Group("/reports")
			{
				reports.POST("/mileage", deps.FleetHandler.MileageReport)
				reports.POST("/overspeed", deps.FleetHandler.OverspeedReport)
				reports.POST("/idle", deps.FleetHandler.IdleReport)
			}
		}
	}
}

// Start begins listening for HTTP connections
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.APIHost, s.cfg.APIPort)

	s.srv = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.log.Info("API server starting", zap.String("addr", addr))

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
	}()

	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server: %w", err)
	}

	return nil
}
