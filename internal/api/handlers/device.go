package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/h2o/gps-platform/internal/storage/postgres"
	"github.com/h2o/gps-platform/internal/storage/redis"
	"go.uber.org/zap"
)

// DeviceHandler serves CRUD endpoints for GPS devices
type DeviceHandler struct {
	db    *postgres.DB
	cache *redis.Cache
	log   *zap.Logger
}

func NewDeviceHandler(db *postgres.DB, cache *redis.Cache, log *zap.Logger) *DeviceHandler {
	return &DeviceHandler{db: db, cache: cache, log: log}
}

type deviceResponse struct {
	ID          string     `json:"id"`
	IMEI        string     `json:"imei"`
	VehicleID   *string    `json:"vehicle_id"`
	Model       string     `json:"model"`
	SimNumber   string     `json:"sim_number"`
	SimOperator string     `json:"sim_operator"`
	IsActive    bool       `json:"is_active"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	InstalledAt *time.Time `json:"installed_at"`
}

// List returns all devices for the authenticated tenant (paginated)
func (h *DeviceHandler) List(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	page := 1
	pageSize := 50

	ctx := h.db.WithTenant(c.Request.Context(), tenantID)

	rows, err := h.db.Pool().Query(ctx, `
		SELECT id, imei, vehicle_id, model, sim_number, sim_operator,
		       is_active, last_seen_at, installed_at
		FROM devices
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, pageSize, (page-1)*pageSize)

	if err != nil {
		h.log.Error("list devices", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	devices := make([]deviceResponse, 0, pageSize)
	for rows.Next() {
		var d deviceResponse
		if err := rows.Scan(&d.ID, &d.IMEI, &d.VehicleID, &d.Model,
			&d.SimNumber, &d.SimOperator, &d.IsActive,
			&d.LastSeenAt, &d.InstalledAt); err != nil {
			continue
		}
		devices = append(devices, d)
	}

	c.JSON(http.StatusOK, gin.H{
		"devices": devices,
		"count":   len(devices),
		"page":    page,
	})
}

type createDeviceRequest struct {
	IMEI        string  `json:"imei" binding:"required,len=15"`
	VehicleID   *string `json:"vehicle_id"`
	Model       string  `json:"model"`
	SimNumber   string  `json:"sim_number"`
	SimOperator string  `json:"sim_operator"`
}

// Create registers a new GPS device for the tenant
func (h *DeviceHandler) Create(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	var req createDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var id string
	err := h.db.Pool().QueryRow(c.Request.Context(), `
		INSERT INTO devices(tenant_id, vehicle_id, imei, model, sim_number, sim_operator)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, tenantID, req.VehicleID, req.IMEI, req.Model, req.SimNumber, req.SimOperator).Scan(&id)

	if err != nil {
		h.log.Error("create device", zap.Error(err))
		c.JSON(http.StatusConflict, gin.H{"error": "device already exists or invalid data"})
		return
	}

	// Invalidate tenant device cache
	_ = h.cache.GetLivePosition(c.Request.Context(), req.IMEI) // warm cache

	h.log.Info("device registered",
		zap.String("id", id),
		zap.String("imei", req.IMEI),
		zap.String("tenant", tenantID))

	c.JSON(http.StatusCreated, gin.H{"id": id, "imei": req.IMEI})
}

// Get returns details for a single device by IMEI
func (h *DeviceHandler) Get(c *gin.Context) {
	imei := c.Param("imei")
	tenantID := c.GetString("tenant_id")

	var d deviceResponse
	err := h.db.Pool().QueryRow(c.Request.Context(), `
		SELECT id, imei, vehicle_id, model, sim_number, sim_operator,
		       is_active, last_seen_at, installed_at
		FROM devices
		WHERE imei = $1 AND tenant_id = $2
	`, imei, tenantID).Scan(&d.ID, &d.IMEI, &d.VehicleID, &d.Model,
		&d.SimNumber, &d.SimOperator, &d.IsActive, &d.LastSeenAt, &d.InstalledAt)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	c.JSON(http.StatusOK, d)
}

// Update modifies device attributes
func (h *DeviceHandler) Update(c *gin.Context) {
	imei := c.Param("imei")
	tenantID := c.GetString("tenant_id")

	var req struct {
		VehicleID   *string `json:"vehicle_id"`
		Model       string  `json:"model"`
		IsActive    *bool   `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.db.Pool().Exec(c.Request.Context(), `
		UPDATE devices
		SET vehicle_id = COALESCE($1, vehicle_id),
		    model      = COALESCE(NULLIF($2,''), model),
		    is_active  = COALESCE($3, is_active),
		    updated_at = NOW()
		WHERE imei = $4 AND tenant_id = $5
	`, req.VehicleID, req.Model, req.IsActive, imei, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}

	// Bust device info cache
	_ = h.cache.GetLivePosition(c.Request.Context(), imei)

	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// Delete soft-deletes a device (sets is_active = false)
func (h *DeviceHandler) Delete(c *gin.Context) {
	imei := c.Param("imei")
	tenantID := c.GetString("tenant_id")

	_, err := h.db.Pool().Exec(c.Request.Context(),
		"UPDATE devices SET is_active = false, updated_at = NOW() WHERE imei = $1 AND tenant_id = $2",
		imei, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "device deactivated"})
}
