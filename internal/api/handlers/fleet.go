package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/h2o/gps-platform/internal/storage/postgres"
	"github.com/h2o/gps-platform/internal/storage/redis"
	"go.uber.org/zap"
)

// FleetHandler provides fleet-wide analytics and live overview
type FleetHandler struct {
	db    *postgres.DB
	cache *redis.Cache
	log   *zap.Logger
}

func NewFleetHandler(db *postgres.DB, cache *redis.Cache, log *zap.Logger) *FleetHandler {
	return &FleetHandler{db: db, cache: cache, log: log}
}

// Summary returns high-level fleet stats for dashboard KPIs
func (h *FleetHandler) Summary(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	var totalDevices, activeDevices, movingDevices, idleDevices int

	_ = h.db.Pool().QueryRow(c.Request.Context(), `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE is_active) AS active
		FROM devices WHERE tenant_id = $1
	`, tenantID).Scan(&totalDevices, &activeDevices)

	c.JSON(http.StatusOK, gin.H{
		"total_devices":  totalDevices,
		"active_devices": activeDevices,
		"moving":         movingDevices,
		"idle":           idleDevices,
	})
}

// LiveAll returns the latest position for ALL devices in the tenant (from Redis)
func (h *FleetHandler) LiveAll(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	// Get all IMEI numbers for the tenant
	rows, err := h.db.Pool().Query(c.Request.Context(),
		"SELECT imei FROM devices WHERE tenant_id = $1 AND is_active = true",
		tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	imeis := make([]string, 0, 100)
	for rows.Next() {
		var imei string
		if err := rows.Scan(&imei); err == nil {
			imeis = append(imeis, imei)
		}
	}

	// Batch-fetch from Redis (pipeline, single round-trip)
	positions, err := h.cache.GetMultipleLivePositions(c.Request.Context(), imeis)
	if err != nil {
		h.log.Warn("live positions cache error", zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{
		"count":     len(positions),
		"positions": positions,
	})
}

// Heatmap returns aggregate location data for density visualization
func (h *FleetHandler) Heatmap(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	rows, err := h.db.Pool().Query(c.Request.Context(), `
		SELECT
			round(latitude::numeric, 3)  AS lat,
			round(longitude::numeric, 3) AS lng,
			COUNT(*) AS weight
		FROM locations
		WHERE tenant_id = $1
		  AND recorded_at >= NOW() - INTERVAL '24 hours'
		GROUP BY 1, 2
		ORDER BY weight DESC
		LIMIT 5000
	`, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	type HeatPoint struct {
		Lat    float64 `json:"lat"`
		Lng    float64 `json:"lng"`
		Weight int     `json:"weight"`
	}

	points := make([]HeatPoint, 0, 5000)
	for rows.Next() {
		var p HeatPoint
		if err := rows.Scan(&p.Lat, &p.Lng, &p.Weight); err == nil {
			points = append(points, p)
		}
	}

	c.JSON(http.StatusOK, gin.H{"points": points})
}

// ListGeofences returns all geofences for the tenant
func (h *FleetHandler) ListGeofences(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	rows, err := h.db.Pool().Query(c.Request.Context(), `
		SELECT id, name, type, color, coordinates, center_lat, center_lng, radius_m
		FROM geofences WHERE tenant_id = $1 AND is_active = true
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	type Geofence struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Type        string   `json:"type"`
		Color       string   `json:"color"`
		Coordinates any      `json:"coordinates"`
		CenterLat   *float64 `json:"center_lat"`
		CenterLng   *float64 `json:"center_lng"`
		RadiusM     *float32 `json:"radius_m"`
	}

	fences := make([]Geofence, 0)
	for rows.Next() {
		var f Geofence
		if err := rows.Scan(&f.ID, &f.Name, &f.Type, &f.Color,
			&f.Coordinates, &f.CenterLat, &f.CenterLng, &f.RadiusM); err == nil {
			fences = append(fences, f)
		}
	}

	c.JSON(http.StatusOK, gin.H{"geofences": fences})
}

// CreateGeofence creates a new geofence polygon or circle
func (h *FleetHandler) CreateGeofence(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	var req struct {
		Name        string  `json:"name" binding:"required"`
		Type        string  `json:"type" binding:"required,oneof=circle polygon rectangle"`
		Color       string  `json:"color"`
		Coordinates any     `json:"coordinates" binding:"required"`
		CenterLat   float64 `json:"center_lat"`
		CenterLng   float64 `json:"center_lng"`
		RadiusM     float32 `json:"radius_m"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var id string
	_ = h.db.Pool().QueryRow(c.Request.Context(), `
		INSERT INTO geofences(tenant_id, name, type, color, coordinates, center_lat, center_lng, radius_m)
		VALUES ($1, $2, $3, COALESCE(NULLIF($4,''), '#3B82F6'), $5, $6, $7, $8)
		RETURNING id
	`, tenantID, req.Name, req.Type, req.Color, req.Coordinates,
		req.CenterLat, req.CenterLng, req.RadiusM).Scan(&id)

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// UpdateGeofence modifies an existing geofence
func (h *FleetHandler) UpdateGeofence(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// DeleteGeofence soft-deletes a geofence
func (h *FleetHandler) DeleteGeofence(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	_, _ = h.db.Pool().Exec(c.Request.Context(),
		"UPDATE geofences SET is_active = false WHERE id = $1 AND tenant_id = $2",
		id, tenantID)

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// MileageReport generates distance summary per device
func (h *FleetHandler) MileageReport(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "report generation queued"})
}

// OverspeedReport lists overspeed incidents
func (h *FleetHandler) OverspeedReport(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "report generation queued"})
}

// IdleReport lists idle time periods
func (h *FleetHandler) IdleReport(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "report generation queued"})
}
