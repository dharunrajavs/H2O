package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 50 * time.Second // must be < pongWait
	maxMessageSize = 4096
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // origin checked via JWT auth middleware
	},
}

// Client represents a single WebSocket connection (one browser tab or mobile session)
type Client struct {
	ID       string
	TenantID string
	UserID   string

	hub  *Hub
	conn *websocket.Conn
	send chan []byte // buffered outbound messages

	log *zap.Logger
}

// ClientMessage is the structure of messages received from the client
type ClientMessage struct {
	Action  string   `json:"action"`   // "subscribe", "unsubscribe", "ping"
	Devices []string `json:"devices"`  // list of device IDs to subscribe to
}

// ServeWS upgrades an HTTP connection to WebSocket and starts the client loops
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request,
	clientID, tenantID, userID string, log *zap.Logger) {

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("websocket upgrade failed", zap.Error(err))
		return
	}

	client := &Client{
		ID:       clientID,
		TenantID: tenantID,
		UserID:   userID,
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 512),
		log:      log.With(zap.String("ws_client", clientID)),
	}

	hub.register <- client

	go client.writePump()
	go client.readPump(r.Context())
}

// readPump reads control messages from the client (subscribe/unsubscribe)
func (c *Client) readPump(ctx context.Context) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure) {
				c.log.Debug("ws read error", zap.Error(err))
			}
			return
		}

		var cmd ClientMessage
		if err := json.Unmarshal(msg, &cmd); err != nil {
			c.log.Debug("invalid ws message", zap.Error(err))
			continue
		}

		switch cmd.Action {
		case "subscribe":
			if len(cmd.Devices) > 0 {
				c.hub.Subscribe(c, cmd.Devices)
				c.sendJSON(map[string]any{
					"type":    "subscribed",
					"devices": cmd.Devices,
				})
			}
		case "unsubscribe":
			// TODO: implement selective unsubscribe
		case "ping":
			c.sendJSON(map[string]any{"type": "pong"})
		}
	}
}

// writePump drains the send channel and writes messages to the WebSocket
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) sendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}
