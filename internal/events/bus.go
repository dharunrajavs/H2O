// Package events provides a zero-copy, in-process typed pub-sub bus.
// In a monolith, components communicate through typed Go channels instead of
// a network broker (Redis Streams). This gives sub-microsecond latency and
// removes an entire network hop from the hot path.
//
// For cross-instance WebSocket fanout (when you scale to N pods), Redis
// Pub/Sub is still used — but that is in the websocket package, not here.
package events

import (
	"sync"
	"time"
)

// ─── Domain event types ───────────────────────────────────────────────────────

// GPSEvent is published every time a device sends a GT06 location packet
type GPSEvent struct {
	IMEI         string
	TenantID     string
	DeviceID     string
	Latitude     float64
	Longitude    float64
	Speed        float64 // km/h
	Heading      float64 // degrees 0–360
	Satellites   int
	IsGPSFixed   bool
	Ignition     bool
	AlarmType    uint8
	MCC          uint16
	MNC          uint8
	LAC          uint16
	CellID       uint32
	RecordedAt   time.Time // timestamp from the device packet
	ReceivedAt   time.Time // timestamp of server receive
	PacketSerial uint16
}

// AlarmEvent is published when the device reports any alarm condition
type AlarmEvent struct {
	GPSEvent
	AlarmCode uint8
	AlarmName string
}

// LoginEvent is published once when a device successfully authenticates
type LoginEvent struct {
	IMEI       string
	TenantID   string
	DeviceID   string
	RemoteAddr string
	LoginAt    time.Time
}

// HeartbeatEvent is published on each device keepalive packet
type HeartbeatEvent struct {
	IMEI         string
	TenantID     string
	VoltageLevel uint8
	GSMSignal    uint8
	ReceivedAt   time.Time
}

// ─── Generic typed topic ──────────────────────────────────────────────────────

// Topic is a typed pub-sub channel that fans out to N subscribers.
// Publishing never blocks: slow subscribers drop messages (back-pressure handled
// by the subscriber's channel buffer size).
type Topic[T any] struct {
	mu   sync.RWMutex
	subs []chan T
}

// Subscribe returns a receive-only channel buffered to bufSize events.
// The caller must drain the channel to avoid silent drops.
func (t *Topic[T]) Subscribe(bufSize int) <-chan T {
	ch := make(chan T, bufSize)
	t.mu.Lock()
	t.subs = append(t.subs, ch)
	t.mu.Unlock()
	return ch
}

// Publish fans the event out to all subscribers. Non-blocking: if a subscriber's
// buffer is full the event is dropped for that subscriber only.
func (t *Topic[T]) Publish(v T) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, ch := range t.subs {
		select {
		case ch <- v:
		default: // subscriber is slow — drop rather than block the TCP read loop
		}
	}
}

// Close signals all subscribers that no more events will come.
func (t *Topic[T]) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, ch := range t.subs {
		close(ch)
	}
	t.subs = nil
}

// ─── Bus ─────────────────────────────────────────────────────────────────────

// Bus holds all typed topics used within the monolith.
// Wire it once at startup and inject it wherever needed.
type Bus struct {
	GPS       Topic[*GPSEvent]
	Alarm     Topic[*AlarmEvent]
	Login     Topic[*LoginEvent]
	Heartbeat Topic[*HeartbeatEvent]
}

// Close gracefully shuts down all topics.
func (b *Bus) Close() {
	b.GPS.Close()
	b.Alarm.Close()
	b.Login.Close()
	b.Heartbeat.Close()
}
