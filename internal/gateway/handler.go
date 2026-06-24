package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/h2o/gps-platform/internal/events"
	"github.com/h2o/gps-platform/internal/protocol/gt06"
	"github.com/h2o/gps-platform/internal/storage/redis"
	"go.uber.org/zap"
)

// DeviceResolver looks up tenant/device info by IMEI (cache-first, DB fallback)
type DeviceResolver interface {
	ResolveByIMEI(ctx context.Context, imei string) (tenantID, deviceID string, err error)
}

// Handler processes decoded GT06 packets and publishes to the in-process event bus.
// Publishing is a non-blocking channel send — zero serialisation cost.
type Handler struct {
	resolver DeviceResolver
	bus      *events.Bus
	cache    *redis.Cache
	log      *zap.Logger
	decoder  *gt06.Decoder
}

func NewHandler(resolver DeviceResolver, bus *events.Bus, cache *redis.Cache, log *zap.Logger) *Handler {
	return &Handler{
		resolver: resolver,
		bus:      bus,
		cache:    cache,
		log:      log,
		decoder:  gt06.NewDecoder(),
	}
}

// HandlePacket routes a decoded packet to the appropriate handler
func (h *Handler) HandlePacket(ctx context.Context, conn *Connection, pkt *gt06.RawPacket) {
	var err error
	switch pkt.ProtocolNum {
	case gt06.ProtoLogin:
		err = h.handleLogin(ctx, conn, pkt)
	case gt06.ProtoGPS, gt06.ProtoGPSShort:
		err = h.handleGPS(ctx, conn, pkt)
	case gt06.ProtoHeartbeat:
		err = h.handleHeartbeat(ctx, conn, pkt)
	case gt06.ProtoAlarm:
		err = h.handleAlarm(ctx, conn, pkt)
	case gt06.ProtoExternalPower:
		_ = gt06.BuildServerResponse(pkt.ProtocolNum, pkt.SerialNumber)
		conn.Send(gt06.BuildServerResponse(pkt.ProtocolNum, pkt.SerialNumber))
	default:
		h.log.Debug("unknown GT06 protocol number",
			zap.Uint8("proto", pkt.ProtocolNum),
			zap.String("imei", conn.session.IMEI))
	}
	if err != nil {
		h.log.Error("packet handling error",
			zap.Error(err),
			zap.Uint8("proto", pkt.ProtocolNum),
			zap.String("imei", conn.session.IMEI))
	}
}

func (h *Handler) handleLogin(ctx context.Context, conn *Connection, pkt *gt06.RawPacket) error {
	login, err := h.decoder.DecodeLogin(pkt)
	if err != nil {
		return fmt.Errorf("decode login: %w", err)
	}

	tenantID, deviceID, err := h.resolver.ResolveByIMEI(ctx, login.IMEI)
	if err != nil {
		h.log.Warn("rejecting unknown IMEI",
			zap.String("imei", login.IMEI),
			zap.String("remote", conn.session.RemoteAddr))
		conn.Close()
		return nil
	}

	conn.session.IMEI = login.IMEI
	conn.session.TenantID = tenantID
	conn.session.DeviceID = deviceID
	conn.session.LoginAt = time.Now().UTC()
	conn.session.State = StateActive

	if err := h.cache.SetDeviceSession(ctx, login.IMEI, &redis.DeviceSession{
		IMEI:       login.IMEI,
		TenantID:   tenantID,
		DeviceID:   deviceID,
		RemoteAddr: conn.session.RemoteAddr,
		LoginAt:    conn.session.LoginAt,
	}); err != nil {
		h.log.Warn("session cache write failed", zap.Error(err))
	}

	conn.Send(gt06.BuildServerResponse(pkt.ProtocolNum, pkt.SerialNumber))

	// In-process publish — no serialisation, no network
	h.bus.Login.Publish(&events.LoginEvent{
		IMEI:       login.IMEI,
		TenantID:   tenantID,
		DeviceID:   deviceID,
		RemoteAddr: conn.session.RemoteAddr,
		LoginAt:    conn.session.LoginAt,
	})

	h.log.Info("device login",
		zap.String("imei", login.IMEI),
		zap.String("tenant", tenantID),
		zap.String("device", deviceID))
	return nil
}

func (h *Handler) handleGPS(ctx context.Context, conn *Connection, pkt *gt06.RawPacket) error {
	if conn.session.State != StateActive {
		return fmt.Errorf("GPS from unauthenticated device %s", conn.session.IMEI)
	}

	gpsPkt, err := h.decoder.DecodeGPS(pkt)
	if err != nil {
		return fmt.Errorf("decode GPS: %w", err)
	}

	evt := &events.GPSEvent{
		IMEI:         conn.session.IMEI,
		TenantID:     conn.session.TenantID,
		DeviceID:     conn.session.DeviceID,
		Latitude:     gpsPkt.Latitude,
		Longitude:    gpsPkt.Longitude,
		Speed:        gpsPkt.Speed,
		Heading:      gpsPkt.Heading,
		Satellites:   gpsPkt.Satellites,
		IsGPSFixed:   gpsPkt.IsGPSFixed,
		Ignition:     gpsPkt.Ignition,
		MCC:          gpsPkt.MCC,
		MNC:          gpsPkt.MNC,
		LAC:          gpsPkt.LAC,
		CellID:       gpsPkt.CellID,
		RecordedAt:   gpsPkt.Timestamp,
		ReceivedAt:   time.Now().UTC(),
		PacketSerial: pkt.SerialNumber,
	}

	// Write live position to Redis immediately (< 1ms) for API reads
	_ = h.cache.SetLivePosition(ctx, evt)

	// Fan out to all in-process subscribers (storage worker, WS broadcaster, alert engine)
	h.bus.GPS.Publish(evt)

	conn.Send(gt06.BuildServerResponse(pkt.ProtocolNum, pkt.SerialNumber))
	return nil
}

func (h *Handler) handleHeartbeat(ctx context.Context, conn *Connection, pkt *gt06.RawPacket) error {
	if conn.session.State != StateActive {
		return nil
	}
	hb, err := h.decoder.DecodeHeartbeat(pkt)
	if err != nil {
		return fmt.Errorf("decode heartbeat: %w", err)
	}

	h.bus.Heartbeat.Publish(&events.HeartbeatEvent{
		IMEI:         conn.session.IMEI,
		TenantID:     conn.session.TenantID,
		VoltageLevel: hb.VoltageLevel,
		GSMSignal:    hb.GSMSignal,
		ReceivedAt:   hb.Timestamp,
	})

	_ = h.cache.RefreshSession(ctx, conn.session.IMEI)
	conn.Send(gt06.BuildServerResponse(pkt.ProtocolNum, pkt.SerialNumber))
	return nil
}

func (h *Handler) handleAlarm(ctx context.Context, conn *Connection, pkt *gt06.RawPacket) error {
	if conn.session.State != StateActive {
		return nil
	}
	gpsPkt, err := h.decoder.DecodeGPS(pkt)
	if err != nil {
		return fmt.Errorf("decode alarm GPS: %w", err)
	}

	evt := &events.AlarmEvent{
		GPSEvent: events.GPSEvent{
			IMEI:       conn.session.IMEI,
			TenantID:   conn.session.TenantID,
			DeviceID:   conn.session.DeviceID,
			Latitude:   gpsPkt.Latitude,
			Longitude:  gpsPkt.Longitude,
			Speed:      gpsPkt.Speed,
			Heading:    gpsPkt.Heading,
			Ignition:   gpsPkt.Ignition,
			RecordedAt: gpsPkt.Timestamp,
			ReceivedAt: time.Now().UTC(),
		},
		AlarmCode: gpsPkt.AlarmType,
		AlarmName: gt06.AlarmName(gpsPkt.AlarmType),
	}

	h.bus.Alarm.Publish(evt)
	conn.Send(gt06.BuildServerResponse(pkt.ProtocolNum, pkt.SerialNumber))

	h.log.Warn("device alarm",
		zap.String("imei", conn.session.IMEI),
		zap.String("alarm", evt.AlarmName))
	return nil
}
