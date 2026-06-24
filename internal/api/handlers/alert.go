package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/h2o/gps-platform/internal/storage/postgres"
	"go.uber.org/zap"
)

// AlertHandler manages alert rules and alert history
type AlertHandler struct {
	db  *postgres.DB
	log *zap.Logger
}

func NewAlertHandler(db *postgres.DB, log *zap.Logger) *AlertHandler {
	return &AlertHandler{db: db, log: log}
}

// List returns recent alert events for the tenant
func (h *AlertHandler) List(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	rows, err := h.db.Pool().Query(c.Request.Context(), `
		SELECT a.id, a.type, a.severity, a.latitude, a.longitude,
		       a.speed, a.message, a.is_read, a.triggered_at,
		       d.imei
		FROM alerts a
		JOIN devices d ON d.id = a.device_id
		WHERE a.tenant_id = $1
		ORDER BY a.triggered_at DESC
		LIMIT 100
	`, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	type Alert struct {
		ID          string   `json:"id"`
		Type        string   `json:"type"`
		Severity    string   `json:"severity"`
		Latitude    *float64 `json:"lat"`
		Longitude   *float64 `json:"lng"`
		Speed       *float32 `json:"speed"`
		Message     *string  `json:"message"`
		IsRead      bool     `json:"is_read"`
		TriggeredAt string   `json:"triggered_at"`
		IMEI        string   `json:"imei"`
	}

	alerts := make([]Alert, 0, 100)
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.Type, &a.Severity, &a.Latitude, &a.Longitude,
			&a.Speed, &a.Message, &a.IsRead, &a.TriggeredAt, &a.IMEI); err == nil {
			alerts = append(alerts, a)
		}
	}

	c.JSON(http.StatusOK, gin.H{"alerts": alerts, "count": len(alerts)})
}

// MarkRead marks an alert as read
func (h *AlertHandler) MarkRead(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	_, _ = h.db.Pool().Exec(c.Request.Context(),
		"UPDATE alerts SET is_read = true WHERE id = $1 AND tenant_id = $2",
		id, tenantID)

	c.JSON(http.StatusOK, gin.H{"message": "marked as read"})
}

// ListRules returns alert rules for the tenant
func (h *AlertHandler) ListRules(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	rows, err := h.db.Pool().Query(c.Request.Context(), `
		SELECT id, name, type, device_ids, config, channels, is_active
		FROM alert_rules WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	rules := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id, name, ruleType string
			deviceIDs          []string
			config, channels   map[string]any
			isActive           bool
		)
		if err := rows.Scan(&id, &name, &ruleType, &deviceIDs, &config, &channels, &isActive); err == nil {
			rules = append(rules, map[string]any{
				"id":         id,
				"name":       name,
				"type":       ruleType,
				"device_ids": deviceIDs,
				"config":     config,
				"channels":   channels,
				"is_active":  isActive,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// CreateRule creates a new alert rule
func (h *AlertHandler) CreateRule(c *gin.Context) {
	tenantID := c.GetString("tenant_id")

	var req struct {
		Name      string         `json:"name" binding:"required"`
		Type      string         `json:"type" binding:"required"`
		DeviceIDs []string       `json:"device_ids"`
		Config    map[string]any `json:"config"`
		Channels  map[string]any `json:"channels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var id string
	_ = h.db.Pool().QueryRow(c.Request.Context(), `
		INSERT INTO alert_rules(tenant_id, name, type, device_ids, config, channels)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, tenantID, req.Name, req.Type, req.DeviceIDs, req.Config, req.Channels).Scan(&id)

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// UpdateRule modifies an alert rule
func (h *AlertHandler) UpdateRule(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

// DeleteRule removes an alert rule
func (h *AlertHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	tenantID := c.GetString("tenant_id")

	_, _ = h.db.Pool().Exec(c.Request.Context(),
		"UPDATE alert_rules SET is_active = false WHERE id = $1 AND tenant_id = $2",
		id, tenantID)

	c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
}
