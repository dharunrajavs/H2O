package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/h2o/gps-platform/internal/storage/postgres"
	"github.com/h2o/gps-platform/internal/storage/redis"
	"go.uber.org/zap"
)

// LocationHandler serves location history and live position endpoints
type LocationHandler struct {
	db    *postgres.DB
	cache *redis.Cache
	log   *zap.Logger
}

func NewLocationHandler(db *postgres.DB, cache *redis.Cache, log *zap.Logger) *LocationHandler {
	return &LocationHandler{db: db, cache: cache, log: log}
}

// GetLive returns the latest position for a device from Redis (< 1ms)
func (h *LocationHandler) GetLive(c *gin.Context) {
	imei := c.Param("imei")

	pos, err := h.cache.GetLivePosition(c.Request.Context(), imei)
	if err != nil {
		h.log.Error("get live position", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cache error"})
		return
	}

	if pos == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no live data for device"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"imei":        pos.IMEI,
		"lat":         pos.Latitude,
		"lng":         pos.Longitude,
		"speed":       pos.Speed,
		"heading":     pos.Heading,
		"ignition":    pos.Ignition,
		"gps_fixed":   pos.GPSFixed,
		"recorded_at": pos.RecordedAt,
		"updated_at":  pos.UpdatedAt,
	})
}

type HistoryRequest struct {
	From   time.Time `form:"from" time_format:"2006-01-02T15:04:05Z07:00"`
	To     time.Time `form:"to"   time_format:"2006-01-02T15:04:05Z07:00"`
	Limit  int       `form:"limit,default=1000"`
	Offset int       `form:"offset,default=0"`
}

// GetHistory returns paginated location history from TimescaleDB
func (h *LocationHandler) GetHistory(c *gin.Context) {
	imei := c.Param("imei")
	tenantID := c.GetString("tenant_id")

	var req HistoryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.From.IsZero() {
		req.From = time.Now().Add(-24 * time.Hour)
	}
	if req.To.IsZero() {
		req.To = time.Now()
	}
	if req.Limit > 10000 {
		req.Limit = 10000
	}

	ctx := h.db.WithTenant(c.Request.Context(), tenantID)

	rows, err := h.db.Pool().Query(ctx, `
		SELECT latitude, longitude, speed, heading, ignition, gps_fixed,
		       satellites, recorded_at
		FROM locations
		WHERE imei = $1
		  AND recorded_at BETWEEN $2 AND $3
		ORDER BY recorded_at ASC
		LIMIT $4 OFFSET $5
	`, imei, req.From, req.To, req.Limit, req.Offset)

	if err != nil {
		h.log.Error("history query", zap.Error(err), zap.String("imei", imei))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	type Point struct {
		Lat        float64   `json:"lat"`
		Lng        float64   `json:"lng"`
		Speed      float32   `json:"speed"`
		Heading    float32   `json:"heading"`
		Ignition   bool      `json:"ignition"`
		GPSFixed   bool      `json:"gps_fixed"`
		Satellites int16     `json:"satellites"`
		RecordedAt time.Time `json:"recorded_at"`
	}

	points := make([]Point, 0, req.Limit)
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.Lat, &p.Lng, &p.Speed, &p.Heading,
			&p.Ignition, &p.GPSFixed, &p.Satellites, &p.RecordedAt); err != nil {
			continue
		}
		points = append(points, p)
	}

	c.JSON(http.StatusOK, gin.H{
		"imei":   imei,
		"from":   req.From,
		"to":     req.To,
		"count":  len(points),
		"points": points,
	})
}

// GetTrips returns trip history for a device
func (h *LocationHandler) GetTrips(c *gin.Context) {
	imei := c.Param("imei")
	tenantID := c.GetString("tenant_id")

	ctx := h.db.WithTenant(c.Request.Context(), tenantID)

	rows, err := h.db.Pool().Query(ctx, `
		SELECT t.id, t.start_lat, t.start_lng, t.end_lat, t.end_lng,
		       t.distance_km, t.max_speed, t.avg_speed, t.duration_secs,
		       t.started_at, t.ended_at
		FROM trips t
		JOIN devices d ON d.id = t.device_id
		WHERE d.imei = $1
		ORDER BY t.started_at DESC
		LIMIT 50
	`, imei)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	type Trip struct {
		ID          string    `json:"id"`
		StartLat    float64   `json:"start_lat"`
		StartLng    float64   `json:"start_lng"`
		EndLat      float64   `json:"end_lat"`
		EndLng      float64   `json:"end_lng"`
		DistanceKm  float32   `json:"distance_km"`
		MaxSpeed    float32   `json:"max_speed"`
		AvgSpeed    float32   `json:"avg_speed"`
		DurationSec int       `json:"duration_secs"`
		StartedAt   time.Time `json:"started_at"`
		EndedAt     *time.Time `json:"ended_at"`
	}

	trips := make([]Trip, 0, 50)
	for rows.Next() {
		var t Trip
		if err := rows.Scan(&t.ID, &t.StartLat, &t.StartLng, &t.EndLat, &t.EndLng,
			&t.DistanceKm, &t.MaxSpeed, &t.AvgSpeed, &t.DurationSec,
			&t.StartedAt, &t.EndedAt); err != nil {
			continue
		}
		trips = append(trips, t)
	}

	c.JSON(http.StatusOK, gin.H{"imei": imei, "trips": trips})
}
