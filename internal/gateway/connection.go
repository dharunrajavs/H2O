package gateway

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/h2o/gps-platform/internal/protocol/gt06"
	"go.uber.org/zap"
)

// ConnectionState represents the FSM state of a device connection
type ConnectionState uint8

const (
	StateNew    ConnectionState = iota // TCP connected, waiting for login
	StateActive                        // Logged in, receiving GPS data
	StateClosed                        // Connection closed
)

// Session holds per-connection runtime state
type Session struct {
	IMEI       string
	TenantID   string
	DeviceID   string
	RemoteAddr string
	LoginAt    time.Time
	LastSeen   time.Time
	State      ConnectionState
}

// Connection wraps a single TCP connection from a GPS device
type Connection struct {
	id      string
	conn    net.Conn
	session *Session
	handler *Handler

	writeCh chan []byte   // buffered ACK write channel
	done    chan struct{}  // closed on connection teardown
	once    sync.Once

	log     *zap.Logger
	decoder *gt06.Decoder

	readBuf [4096]byte // pre-allocated read buffer (stack, not heap per pkt)
}

func newConnection(id string, conn net.Conn, handler *Handler, log *zap.Logger) *Connection {
	return &Connection{
		id:      id,
		conn:    conn,
		handler: handler,
		session: &Session{
			RemoteAddr: conn.RemoteAddr().String(),
			State:      StateNew,
		},
		writeCh: make(chan []byte, 64),
		done:    make(chan struct{}),
		log:     log.With(zap.String("conn_id", id), zap.String("remote", conn.RemoteAddr().String())),
		decoder: gt06.NewDecoder(),
	}
}

// Start begins the read and write loops for this connection.
// Both run as goroutines; Start returns immediately.
func (c *Connection) Start(ctx context.Context) {
	go c.readLoop(ctx)
	go c.writeLoop(ctx)
}

// Close gracefully shuts down the connection
func (c *Connection) Close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
		c.session.State = StateClosed
		c.log.Info("connection closed", zap.String("imei", c.session.IMEI))
	})
}

// Send enqueues an ACK packet for async writing
func (c *Connection) Send(pkt []byte) {
	select {
	case c.writeCh <- pkt:
	case <-c.done:
	default:
		c.log.Warn("write channel full, dropping ACK", zap.String("imei", c.session.IMEI))
	}
}

// readLoop reads raw bytes from TCP and dispatches to the handler
func (c *Connection) readLoop(ctx context.Context) {
	defer c.Close()

	// Rolling buffer for incomplete frame accumulation
	pending := make([]byte, 0, 1024)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		default:
		}

		// Set read deadline to detect stale connections
		_ = c.conn.SetReadDeadline(time.Now().Add(180 * time.Second))

		n, err := c.conn.Read(c.readBuf[:])
		if err != nil {
			if err != io.EOF {
				c.log.Debug("read error", zap.Error(err), zap.String("imei", c.session.IMEI))
			}
			return
		}

		c.session.LastSeen = time.Now()
		pending = append(pending, c.readBuf[:n]...)

		// Process all complete frames in the buffer
		for len(pending) >= 5 {
			pkt, consumed, err := c.decoder.DecodeFrame(pending)
			if err != nil {
				// Check if we need more data
				if isPartialPacketErr(err) {
					break
				}
				c.log.Warn("frame decode error", zap.Error(err),
					zap.String("imei", c.session.IMEI))
				// Skip one byte and try re-sync
				pending = pending[1:]
				continue
			}

			c.handler.HandlePacket(ctx, c, pkt)
			pending = pending[consumed:]
		}

		// Prevent unbounded buffer growth
		if len(pending) > 8192 {
			c.log.Error("pending buffer overflow, resetting",
				zap.String("imei", c.session.IMEI),
				zap.Int("size", len(pending)))
			pending = pending[:0]
		}
	}
}

// writeLoop drains the write channel and sends bytes to device
func (c *Connection) writeLoop(ctx context.Context) {
	defer c.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		case pkt, ok := <-c.writeCh:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := c.conn.Write(pkt); err != nil {
				c.log.Warn("write error", zap.Error(err), zap.String("imei", c.session.IMEI))
				return
			}
		}
	}
}

func isPartialPacketErr(err error) bool {
	if e, ok := err.(*gt06.ErrInvalidPacket); ok {
		return e.Reason == "buffer too short" || e.Reason == "incomplete packet in buffer"
	}
	return false
}
