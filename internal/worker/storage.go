// Package worker contains the background goroutines that run inside the monolith.
// Each worker subscribes to an in-process topic and does one focused job.
package worker

import (
	"context"
	"time"

	"github.com/h2o/gps-platform/internal/events"
	"github.com/h2o/gps-platform/internal/storage/postgres"
	"go.uber.org/zap"
)

// StorageWorker drains the GPS topic and batch-inserts rows into TimescaleDB.
// It collects up to batchSize events or flushes every flushInterval, whichever
// comes first — this absorbs bursts while keeping write latency predictable.
type StorageWorker struct {
	db            *postgres.DB
	bus           *events.Bus
	batchSize     int
	flushInterval time.Duration
	log           *zap.Logger
}

func NewStorageWorker(db *postgres.DB, bus *events.Bus, log *zap.Logger) *StorageWorker {
	return &StorageWorker{
		db:            db,
		bus:           bus,
		batchSize:     500,
		flushInterval: 100 * time.Millisecond,
		log:           log,
	}
}

// Run starts the storage worker. It blocks until ctx is cancelled.
func (w *StorageWorker) Run(ctx context.Context) error {
	// Subscribe with a large buffer so the TCP goroutines never block
	gpsCh := w.bus.GPS.Subscribe(8192)

	batch := make([]postgres.LocationRow, 0, w.batchSize)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		rows := batch
		batch = make([]postgres.LocationRow, 0, w.batchSize)

		if err := w.db.BulkInsertLocations(ctx, rows); err != nil {
			w.log.Error("bulk insert failed",
				zap.Error(err),
				zap.Int("rows", len(rows)))
		} else {
			w.log.Debug("location batch flushed", zap.Int("rows", len(rows)))
		}
	}

	for {
		select {
		case <-ctx.Done():
			// Drain remaining events on shutdown
			for {
				select {
				case evt, ok := <-gpsCh:
					if !ok {
						flush()
						return nil
					}
					batch = append(batch, toRow(evt))
				default:
					flush()
					return nil
				}
			}

		case evt, ok := <-gpsCh:
			if !ok {
				flush()
				return nil
			}
			batch = append(batch, toRow(evt))
			if len(batch) >= w.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

func toRow(e *events.GPSEvent) postgres.LocationRow {
	return postgres.LocationRow{
		TenantID:     e.TenantID,
		DeviceID:     e.DeviceID,
		IMEI:         e.IMEI,
		Latitude:     e.Latitude,
		Longitude:    e.Longitude,
		Speed:        float32(e.Speed),
		Heading:      float32(e.Heading),
		Satellites:   int16(e.Satellites),
		GPSFixed:     e.IsGPSFixed,
		Ignition:     e.Ignition,
		MCC:          int16(e.MCC),
		MNC:          int16(e.MNC),
		LAC:          int32(e.LAC),
		CellID:       int64(e.CellID),
		PacketSerial: int16(e.PacketSerial),
		RecordedAt:   e.RecordedAt,
		ReceivedAt:   e.ReceivedAt,
	}
}
