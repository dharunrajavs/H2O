package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/h2o/gps-platform/internal/events"
	internws "github.com/h2o/gps-platform/internal/websocket"
	"go.uber.org/zap"
)

// BroadcastWorker reads from the GPS topic and pushes JSON to WebSocket clients.
// Because this runs in-process, there is no serialisation between the TCP gateway
// and this worker — the pointer is shared directly until serialisation for the
// WebSocket write.
type BroadcastWorker struct {
	hub *internws.Hub
	bus *events.Bus
	log *zap.Logger
}

func NewBroadcastWorker(hub *internws.Hub, bus *events.Bus, log *zap.Logger) *BroadcastWorker {
	return &BroadcastWorker{hub: hub, bus: bus, log: log}
}

// Run starts the broadcast worker and blocks until ctx is cancelled.
func (w *BroadcastWorker) Run(ctx context.Context) error {
	gpsCh := w.bus.GPS.Subscribe(4096)

	for {
		select {
		case <-ctx.Done():
			return nil

		case evt, ok := <-gpsCh:
			if !ok {
				return nil
			}

			msg, err := buildWSMessage(evt)
			if err != nil {
				w.log.Warn("ws message marshal failed", zap.Error(err))
				continue
			}

			w.hub.BroadcastToDevice(ctx, evt.DeviceID, msg)
		}
	}
}

// wsPayload is the JSON envelope sent to WebSocket clients
type wsPayload struct {
	Type      string    `json:"type"`
	DeviceID  string    `json:"device_id"`
	Timestamp time.Time `json:"ts"`
	Data      wsData    `json:"data"`
}

type wsData struct {
	IMEI       string    `json:"imei"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	Speed      float64   `json:"speed"`
	Heading    float64   `json:"heading"`
	Satellites int       `json:"satellites"`
	GPSFixed   bool      `json:"gps_fixed"`
	Ignition   bool      `json:"ignition"`
	RecordedAt time.Time `json:"recorded_at"`
	ReceivedAt time.Time `json:"received_at"`
}

func buildWSMessage(e *events.GPSEvent) ([]byte, error) {
	return json.Marshal(wsPayload{
		Type:      "location",
		DeviceID:  e.DeviceID,
		Timestamp: time.Now().UTC(),
		Data: wsData{
			IMEI:       e.IMEI,
			Lat:        e.Latitude,
			Lng:        e.Longitude,
			Speed:      e.Speed,
			Heading:    e.Heading,
			Satellites: e.Satellites,
			GPSFixed:   e.IsGPSFixed,
			Ignition:   e.Ignition,
			RecordedAt: e.RecordedAt,
			ReceivedAt: e.ReceivedAt,
		},
	})
}
