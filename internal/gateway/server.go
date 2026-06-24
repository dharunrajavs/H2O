package gateway

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/h2o/gps-platform/internal/config"
	"go.uber.org/zap"
)

// Server is the TCP gateway that accepts GT06 connections from GPS devices
type Server struct {
	cfg      *config.TCPConfig
	handler  *Handler
	log      *zap.Logger

	listener   net.Listener
	connsMu    sync.RWMutex
	conns      map[string]*Connection
	activeConns atomic.Int64

	metrics *GatewayMetrics
}

// GatewayMetrics tracks gateway performance counters
type GatewayMetrics struct {
	TotalConnections  atomic.Int64
	ActiveConnections atomic.Int64
	PacketsReceived   atomic.Int64
	PacketErrors      atomic.Int64
	LoginFailures     atomic.Int64
}

// NewServer creates a new TCP gateway server
func NewServer(cfg *config.TCPConfig, handler *Handler, log *zap.Logger) *Server {
	return &Server{
		cfg:     cfg,
		handler: handler,
		log:     log,
		conns:   make(map[string]*Connection, cfg.MaxConnections),
		metrics: &GatewayMetrics{},
	}
}

// Start begins listening for TCP connections.
// It is a blocking call; use a goroutine to run it concurrently.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	lc := net.ListenConfig{
		KeepAlive: s.cfg.KeepAlive,
	}

	var err error
	s.listener, err = lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen TCP %s: %w", addr, err)
	}

	s.log.Info("TCP gateway listening",
		zap.String("addr", addr),
		zap.Int("max_conns", s.cfg.MaxConnections))

	go s.acceptLoop(ctx)
	go s.metricsReporter(ctx)

	<-ctx.Done()
	return s.shutdown()
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				s.log.Error("accept error", zap.Error(err))
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		// Enforce max connection limit
		if int(s.activeConns.Load()) >= s.cfg.MaxConnections {
			s.log.Warn("max connections reached, rejecting",
				zap.String("remote", conn.RemoteAddr().String()))
			_ = conn.Close()
			continue
		}

		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	// Apply TCP socket options for performance
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)     // disable Nagle for ACK latency
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(s.cfg.KeepAlive)
		_ = tc.SetReadBuffer(s.cfg.ReadBufferSize)
		_ = tc.SetWriteBuffer(s.cfg.WriteBufferSize)
	}

	id := uuid.New().String()
	c := newConnection(id, conn, s.handler, s.log)

	s.connsMu.Lock()
	s.conns[id] = c
	s.connsMu.Unlock()

	s.activeConns.Add(1)
	s.metrics.TotalConnections.Add(1)
	s.metrics.ActiveConnections.Add(1)

	s.log.Debug("new connection",
		zap.String("id", id),
		zap.String("remote", conn.RemoteAddr().String()),
		zap.Int64("active", s.activeConns.Load()))

	c.Start(ctx)

	// Block until connection is done (c.done closed)
	<-c.done

	s.connsMu.Lock()
	delete(s.conns, id)
	s.connsMu.Unlock()

	s.activeConns.Add(-1)
	s.metrics.ActiveConnections.Add(-1)
}

func (s *Server) shutdown() error {
	s.log.Info("shutting down TCP gateway")

	if s.listener != nil {
		_ = s.listener.Close()
	}

	s.connsMu.RLock()
	defer s.connsMu.RUnlock()

	for _, c := range s.conns {
		c.Close()
	}

	s.log.Info("TCP gateway stopped")
	return nil
}

func (s *Server) metricsReporter(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.log.Info("gateway metrics",
				zap.Int64("active_connections", s.metrics.ActiveConnections.Load()),
				zap.Int64("total_connections", s.metrics.TotalConnections.Load()),
				zap.Int64("packets_received", s.metrics.PacketsReceived.Load()))
		}
	}
}

// ActiveConnections returns the current number of active TCP connections
func (s *Server) ActiveConnections() int64 {
	return s.activeConns.Load()
}
