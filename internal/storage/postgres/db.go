package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/h2o/gps-platform/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgx connection pool with helper methods
type DB struct {
	pool *pgxpool.Pool
}

// New creates and validates a new PostgreSQL connection pool
func New(ctx context.Context, cfg *config.PostgresConfig) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse postgres DSN: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	poolCfg.MaxConnIdleTime = cfg.ConnMaxIdle
	poolCfg.HealthCheckPeriod = 30 * time.Second

	// Set RLS tenant context after each connection acquisition
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return nil // tenant_id set per-query via BeforeAcquire
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Close shuts down the connection pool
func (db *DB) Close() {
	db.pool.Close()
}

// Pool returns the underlying pgxpool for direct use
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// WithTenant returns a context that sets the RLS tenant_id for the session
func (db *DB) WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

type tenantContextKey struct{}

// TenantFromContext extracts tenant ID from context
func TenantFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(tenantContextKey{}).(string); ok {
		return v
	}
	return ""
}

// Exec runs a query that returns no rows, setting RLS tenant if present in ctx
func (db *DB) Exec(ctx context.Context, sql string, args ...any) error {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if tenantID := TenantFromContext(ctx); tenantID != "" {
		if _, err := conn.Exec(ctx,
			"SELECT set_config('app.current_tenant_id', $1, true)", tenantID); err != nil {
			return fmt.Errorf("set tenant: %w", err)
		}
	}

	_, err = conn.Exec(ctx, sql, args...)
	return err
}

// BulkInsertLocations uses COPY protocol for high-throughput batch inserts
func (db *DB) BulkInsertLocations(ctx context.Context, rows []LocationRow) error {
	if len(rows) == 0 {
		return nil
	}

	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()

	copyCount, err := conn.CopyFrom(
		ctx,
		pgx.Identifier{"locations"},
		[]string{
			"tenant_id", "device_id", "imei",
			"latitude", "longitude", "speed", "heading",
			"satellites", "gps_fixed", "ignition",
			"mcc", "mnc", "lac", "cell_id",
			"packet_serial", "recorded_at", "received_at",
		},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{
				r.TenantID, r.DeviceID, r.IMEI,
				r.Latitude, r.Longitude, r.Speed, r.Heading,
				r.Satellites, r.GPSFixed, r.Ignition,
				r.MCC, r.MNC, r.LAC, r.CellID,
				r.PacketSerial, r.RecordedAt, r.ReceivedAt,
			}, nil
		}),
	)

	if err != nil {
		return fmt.Errorf("COPY locations: %w", err)
	}

	if copyCount != int64(len(rows)) {
		return fmt.Errorf("COPY: expected %d rows, inserted %d", len(rows), copyCount)
	}

	return nil
}

// LocationRow is a flat struct for batch COPY inserts
type LocationRow struct {
	TenantID     string
	DeviceID     string
	IMEI         string
	Latitude     float64
	Longitude    float64
	Speed        float32
	Heading      float32
	Satellites   int16
	GPSFixed     bool
	Ignition     bool
	MCC          int16
	MNC          int16
	LAC          int32
	CellID       int64
	PacketSerial int16
	RecordedAt   time.Time
	ReceivedAt   time.Time
}
