package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Hub manages all active WebSocket client connections and device subscriptions.
// Cross-node fanout is handled via Redis Pub/Sub so all WS nodes receive events
// for subscribed devices regardless of which TCP gateway processed the packet.
type Hub struct {
	clients  map[string]*Client         // clientID → Client
	rooms    map[string]map[string]bool // deviceID → set of clientIDs
	mu       sync.RWMutex

	redis  goredis.UniversalClient
	log    *zap.Logger

	register   chan *Client
	unregister chan *Client
	broadcast  chan *BroadcastMsg
}

// BroadcastMsg is sent to the hub to push an event to all subscribers of a device
type BroadcastMsg struct {
	DeviceID string
	Data     []byte
}

// NewHub creates and initializes a WebSocket hub
func NewHub(redisClient goredis.UniversalClient, log *zap.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		rooms:      make(map[string]map[string]bool),
		redis:      redisClient,
		log:        log,
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
		broadcast:  make(chan *BroadcastMsg, 8192),
	}
}

// Run starts the hub's event loop and Redis subscriber.
// Must be called once before registering any clients.
func (h *Hub) Run(ctx context.Context) {
	go h.redisSubscribeLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			h.mu.Unlock()
			h.log.Debug("client registered", zap.String("id", client.ID))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				// Remove from all rooms
				for deviceID, room := range h.rooms {
					delete(room, client.ID)
					if len(room) == 0 {
						delete(h.rooms, deviceID)
					}
				}
				close(client.send)
			}
			h.mu.Unlock()
			h.log.Debug("client unregistered", zap.String("id", client.ID))

		case msg := <-h.broadcast:
			h.mu.RLock()
			room, ok := h.rooms[msg.DeviceID]
			if ok {
				for clientID := range room {
					if client, exists := h.clients[clientID]; exists {
						select {
						case client.send <- msg.Data:
						default:
							// Slow client: drop message, don't block hub
							h.log.Warn("slow client, dropping message",
								zap.String("client", clientID))
						}
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Subscribe adds a client to one or more device rooms
func (h *Hub) Subscribe(client *Client, deviceIDs []string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, deviceID := range deviceIDs {
		if h.rooms[deviceID] == nil {
			h.rooms[deviceID] = make(map[string]bool)
		}
		h.rooms[deviceID][client.ID] = true
	}

	h.log.Debug("client subscribed",
		zap.String("client", client.ID),
		zap.Strings("devices", deviceIDs))
}

// BroadcastToDevice pushes an event payload to all subscribers of a device.
// Also publishes via Redis Pub/Sub for cross-node delivery.
func (h *Hub) BroadcastToDevice(ctx context.Context, deviceID string, data []byte) {
	// Local delivery
	h.broadcast <- &BroadcastMsg{DeviceID: deviceID, Data: data}

	// Cross-node delivery via Redis Pub/Sub
	channel := fmt.Sprintf("ws:broadcast:%s", deviceID)
	if err := h.redis.Publish(ctx, channel, data).Err(); err != nil {
		h.log.Warn("redis publish error", zap.Error(err))
	}
}

// redisSubscribeLoop subscribes to all ws:broadcast:* channels and delivers
// messages originating from other WS server nodes
func (h *Hub) redisSubscribeLoop(ctx context.Context) {
	sub := h.redis.PSubscribe(ctx, "ws:broadcast:*")
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// Extract deviceID from channel name: "ws:broadcast:{deviceID}"
			deviceID := msg.Channel[len("ws:broadcast:"):]
			payload := []byte(msg.Payload)

			h.mu.RLock()
			room, ok := h.rooms[deviceID]
			if ok {
				for clientID := range room {
					if client, exists := h.clients[clientID]; exists {
						select {
						case client.send <- payload:
						default:
						}
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// ActiveClients returns the number of currently connected WebSocket clients
func (h *Hub) ActiveClients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// WSMessage is the envelope format for WebSocket messages sent to clients
type WSMessage struct {
	Type      string          `json:"type"`
	DeviceID  string          `json:"device_id"`
	Timestamp time.Time       `json:"ts"`
	Data      json.RawMessage `json:"data"`
}

// BuildLocationMessage creates a JSON WebSocket message for a GPS event
func BuildLocationMessage(deviceID string, event any) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	msg := WSMessage{
		Type:      "location",
		DeviceID:  deviceID,
		Timestamp: time.Now().UTC(),
		Data:      json.RawMessage(data),
	}
	return json.Marshal(msg)
}
