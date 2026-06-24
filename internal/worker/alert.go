package worker

import (
	"context"

	"github.com/h2o/gps-platform/internal/events"
	"go.uber.org/zap"
)

// AlertWorker evaluates alarm events against configured rules.
// It subscribes to both the Alarm topic (device-triggered) and the GPS topic
// (for rule-based checks like overspeed or geofence crossing).
type AlertWorker struct {
	bus *events.Bus
	log *zap.Logger
}

func NewAlertWorker(bus *events.Bus, log *zap.Logger) *AlertWorker {
	return &AlertWorker{bus: bus, log: log}
}

// Run starts the alert worker and blocks until ctx is cancelled.
func (w *AlertWorker) Run(ctx context.Context) error {
	alarmCh := w.bus.Alarm.Subscribe(1024)
	gpsCh   := w.bus.GPS.Subscribe(4096)

	for {
		select {
		case <-ctx.Done():
			return nil

		case evt, ok := <-alarmCh:
			if !ok {
				return nil
			}
			w.handleDeviceAlarm(ctx, evt)

		case evt, ok := <-gpsCh:
			if !ok {
				return nil
			}
			w.evaluateRules(ctx, evt)
		}
	}
}

// handleDeviceAlarm processes a hardware alarm raised by the GT06 device
func (w *AlertWorker) handleDeviceAlarm(ctx context.Context, evt *events.AlarmEvent) {
	w.log.Warn("device alarm received",
		zap.String("imei", evt.IMEI),
		zap.String("alarm", evt.AlarmName),
		zap.Float64("lat", evt.Latitude),
		zap.Float64("lng", evt.Longitude))

	// TODO: lookup alert rules for this device/tenant, send push notification,
	// email, SMS via notification service, insert into alerts table
}

// evaluateRules checks a GPS event against tenant-configured rules
// (overspeed, geofence, idle time, etc.)
func (w *AlertWorker) evaluateRules(_ context.Context, evt *events.GPSEvent) {
	// Overspeed check example
	if evt.Speed > 120 {
		w.log.Warn("overspeed detected",
			zap.String("imei", evt.IMEI),
			zap.Float64("speed", evt.Speed))
		// TODO: create alert, push notification
	}

	// TODO: geofence polygon-in-polygon check using PostGIS or in-memory spatial index
	// TODO: idle detection (ignition ON, speed == 0 for > N minutes)
}
