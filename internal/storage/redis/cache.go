package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const (
	ttlLivePosition  = 5 * time.Minute
	ttlDeviceSession = 3 * time.Minute
	ttlDeviceInfo    = 24 * time.Hour
	ttlTenantDevices = time.Minute
)

// Cache provides typed Redis operations for the GPS platform
type Cache struct {
	client goredis.UniversalClient
}

// NewCache creates a new Cache wrapping the given Redis client
func NewCache(client goredis.UniversalClient) *Cache {
	return &Cache{client: client}
}

// ─── Live Position ──────────────────────────────────────────────────────────

// LivePosition is stored as a Redis Hash for instant field reads
type LivePosition struct {
	IMEI       string    `json:"imei"`
	TenantID   string    `json:"tenant_id"`
	DeviceID   string    `json:"device_id"`
	Latitude   float64   `json:"lat"`
	Longitude  float64   `json:"lng"`
	Speed      float64   `json:"speed"`
	Heading    float64   `json:"heading"`
	Ignition   bool      `json:"ignition"`
	GPSFixed   bool      `json:"gps_fixed"`
	RecordedAt time.Time `json:"recorded_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SetLivePosition atomically updates the live position hash and its TTL.
// Uses a pipeline to reduce round-trips.
func (c *Cache) SetLivePosition(ctx context.Context, event interface{}) error {
	type gpsEvent interface {
		getIMEI() string
	}

	// We use a map approach for flexibility with the event type
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	var pos map[string]interface{}
	if err := json.Unmarshal(data, &pos); err != nil {
		return fmt.Errorf("unmarshal event: %w", err)
	}

	imei, _ := pos["imei"].(string)
	if imei == "" {
		return fmt.Errorf("event missing imei")
	}

	key := livePositionKey(imei)

	pipe := c.client.Pipeline()
	pipe.HSet(ctx, key, "imei", imei)
	if lat, ok := pos["lat"]; ok {
		pipe.HSet(ctx, key, "lat", lat)
	}
	if lng, ok := pos["lng"]; ok {
		pipe.HSet(ctx, key, "lng", lng)
	}
	if speed, ok := pos["speed"]; ok {
		pipe.HSet(ctx, key, "speed", speed)
	}
	if heading, ok := pos["heading"]; ok {
		pipe.HSet(ctx, key, "heading", heading)
	}
	if ignition, ok := pos["ignition"]; ok {
		pipe.HSet(ctx, key, "ignition", ignition)
	}
	if gpsFixed, ok := pos["gps_fixed"]; ok {
		pipe.HSet(ctx, key, "gps_fixed", gpsFixed)
	}
	if ts, ok := pos["recorded_at"]; ok {
		pipe.HSet(ctx, key, "recorded_at", ts)
	}
	pipe.HSet(ctx, key, "updated_at", time.Now().UTC().Format(time.RFC3339))
	pipe.Expire(ctx, key, ttlLivePosition)

	_, err = pipe.Exec(ctx)
	return err
}

// GetLivePosition retrieves the latest position for a device
func (c *Cache) GetLivePosition(ctx context.Context, imei string) (*LivePosition, error) {
	key := livePositionKey(imei)

	vals, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("HGETALL %s: %w", key, err)
	}
	if len(vals) == 0 {
		return nil, nil // cache miss
	}

	pos := &LivePosition{
		IMEI: vals["imei"],
	}

	if v, ok := vals["lat"]; ok {
		fmt.Sscanf(v, "%f", &pos.Latitude)
	}
	if v, ok := vals["lng"]; ok {
		fmt.Sscanf(v, "%f", &pos.Longitude)
	}
	if v, ok := vals["speed"]; ok {
		fmt.Sscanf(v, "%f", &pos.Speed)
	}
	if v, ok := vals["heading"]; ok {
		fmt.Sscanf(v, "%f", &pos.Heading)
	}
	if v, ok := vals["ignition"]; ok {
		pos.Ignition = v == "true" || v == "1"
	}
	if v, ok := vals["recorded_at"]; ok {
		pos.RecordedAt, _ = time.Parse(time.RFC3339, v)
	}
	if v, ok := vals["updated_at"]; ok {
		pos.UpdatedAt, _ = time.Parse(time.RFC3339, v)
	}

	return pos, nil
}

// GetMultipleLivePositions fetches live positions for multiple devices using pipeline
func (c *Cache) GetMultipleLivePositions(ctx context.Context, imeis []string) ([]*LivePosition, error) {
	pipe := c.client.Pipeline()
	cmds := make([]*goredis.MapStringStringCmd, len(imeis))

	for i, imei := range imeis {
		cmds[i] = pipe.HGetAll(ctx, livePositionKey(imei))
	}

	if _, err := pipe.Exec(ctx); err != nil && err != goredis.Nil {
		return nil, fmt.Errorf("pipeline HGETALL: %w", err)
	}

	positions := make([]*LivePosition, 0, len(imeis))
	for i, cmd := range cmds {
		vals, _ := cmd.Result()
		if len(vals) == 0 {
			continue
		}
		pos := &LivePosition{IMEI: imeis[i]}
		fmt.Sscanf(vals["lat"], "%f", &pos.Latitude)
		fmt.Sscanf(vals["lng"], "%f", &pos.Longitude)
		fmt.Sscanf(vals["speed"], "%f", &pos.Speed)
		pos.Ignition = vals["ignition"] == "true"
		pos.RecordedAt, _ = time.Parse(time.RFC3339, vals["recorded_at"])
		positions = append(positions, pos)
	}

	return positions, nil
}

// ─── Device Session ─────────────────────────────────────────────────────────

// DeviceSession stores connection metadata for a logged-in device
type DeviceSession struct {
	IMEI       string    `json:"imei"`
	TenantID   string    `json:"tenant_id"`
	DeviceID   string    `json:"device_id"`
	RemoteAddr string    `json:"remote_addr"`
	LoginAt    time.Time `json:"login_at"`
}

// SetDeviceSession stores session info in Redis
func (c *Cache) SetDeviceSession(ctx context.Context, imei string, session *DeviceSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, sessionKey(imei), data, ttlDeviceSession).Err()
}

// GetDeviceSession retrieves session info for an IMEI
func (c *Cache) GetDeviceSession(ctx context.Context, imei string) (*DeviceSession, error) {
	data, err := c.client.Get(ctx, sessionKey(imei)).Bytes()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var session DeviceSession
	return &session, json.Unmarshal(data, &session)
}

// RefreshSession resets the TTL on an existing session
func (c *Cache) RefreshSession(ctx context.Context, imei string) error {
	return c.client.Expire(ctx, sessionKey(imei), ttlDeviceSession).Err()
}

// DeleteSession removes a device session (on disconnect)
func (c *Cache) DeleteSession(ctx context.Context, imei string) error {
	return c.client.Del(ctx, sessionKey(imei)).Err()
}

// ─── Device Info Cache ───────────────────────────────────────────────────────

// DeviceInfo is the tenant+device lookup result cached to avoid DB queries per packet
type DeviceInfo struct {
	TenantID string `json:"tenant_id"`
	DeviceID string `json:"device_id"`
	IsActive bool   `json:"is_active"`
}

// SetDeviceInfo caches device lookup result by IMEI
func (c *Cache) SetDeviceInfo(ctx context.Context, imei string, info *DeviceInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, deviceInfoKey(imei), data, ttlDeviceInfo).Err()
}

// GetDeviceInfo retrieves cached device info by IMEI
func (c *Cache) GetDeviceInfo(ctx context.Context, imei string) (*DeviceInfo, error) {
	data, err := c.client.Get(ctx, deviceInfoKey(imei)).Bytes()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var info DeviceInfo
	return &info, json.Unmarshal(data, &info)
}

// ─── WebSocket Subscriptions ─────────────────────────────────────────────────

// PublishToWebSocket sends a message via Redis Pub/Sub for cross-node WS fanout
func (c *Cache) PublishToWebSocket(ctx context.Context, deviceID string, message []byte) error {
	channel := fmt.Sprintf("ws:broadcast:%s", deviceID)
	return c.client.Publish(ctx, channel, message).Err()
}

// ─── Deduplication ───────────────────────────────────────────────────────────

// IsDuplicate checks if a packet with this (imei, serial) was recently seen.
// Uses SETNX with TTL to track recent serials per device.
func (c *Cache) IsDuplicate(ctx context.Context, imei string, serial uint16) (bool, error) {
	key := fmt.Sprintf("dedup:%s:%d", imei, serial)
	set, err := c.client.SetNX(ctx, key, 1, 60*time.Second).Result()
	if err != nil {
		return false, err
	}
	return !set, nil // if SetNX returned false, key already existed → duplicate
}

// ─── Key builders ────────────────────────────────────────────────────────────

func livePositionKey(imei string) string  { return fmt.Sprintf("device:%s:live", imei) }
func sessionKey(imei string) string       { return fmt.Sprintf("device:%s:session", imei) }
func deviceInfoKey(imei string) string    { return fmt.Sprintf("device:%s:info", imei) }
