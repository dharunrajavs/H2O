package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/h2o/gps-platform/internal/api/middleware"
	"github.com/h2o/gps-platform/internal/storage/postgres"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles login, token refresh, and logout
type AuthHandler struct {
	db            *postgres.DB
	accessSecret  string
	refreshSecret string
	accessTTL     time.Duration
	refreshTTL    time.Duration
	log           *zap.Logger
}

func NewAuthHandler(db *postgres.DB, accessSecret, refreshSecret string,
	accessTTL, refreshTTL time.Duration, log *zap.Logger) *AuthHandler {
	return &AuthHandler{
		db:            db,
		accessSecret:  accessSecret,
		refreshSecret: refreshSecret,
		accessTTL:     accessTTL,
		refreshTTL:    refreshTTL,
		log:           log,
	}
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// Login authenticates a user and returns JWT access + refresh tokens
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fetch user from DB
	var (
		userID       string
		tenantID     string
		role         string
		passwordHash string
		isActive     bool
	)

	err := h.db.Pool().QueryRow(c.Request.Context(), `
		SELECT u.id, u.tenant_id, u.role, u.password_hash, u.is_active
		FROM users u
		WHERE u.email = $1
	`, req.Email).Scan(&userID, &tenantID, &role, &passwordHash, &isActive)

	if err != nil {
		// Constant-time response to prevent user enumeration
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$dummy"), []byte(req.Password))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !isActive {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "account disabled"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	accessToken, err := h.generateToken(userID, tenantID, role,
		h.accessSecret, h.accessTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	refreshToken, err := h.generateToken(userID, tenantID, role,
		h.refreshSecret, h.refreshTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	// Store refresh token hash in DB for revocation
	tokenHash := hashToken(refreshToken)
	_, _ = h.db.Pool().Exec(c.Request.Context(), `
		INSERT INTO refresh_tokens(user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, time.Now().Add(h.refreshTTL))

	// Update last login
	_, _ = h.db.Pool().Exec(c.Request.Context(),
		"UPDATE users SET last_login_at = NOW() WHERE id = $1", userID)

	h.log.Info("user logged in",
		zap.String("user", userID),
		zap.String("tenant", tenantID))

	c.JSON(http.StatusOK, gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    int(h.accessTTL.Seconds()),
		"token_type":    "Bearer",
	})
}

// Refresh issues a new access token given a valid refresh token
func (h *AuthHandler) Refresh(c *gin.Context) {
	var body struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(body.RefreshToken, claims,
		func(t *jwt.Token) (interface{}, error) {
			return []byte(h.refreshSecret), nil
		})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	// Verify not revoked
	tokenHash := hashToken(body.RefreshToken)
	var revokedAt *time.Time
	_ = h.db.Pool().QueryRow(c.Request.Context(),
		"SELECT revoked_at FROM refresh_tokens WHERE token_hash = $1 AND expires_at > NOW()",
		tokenHash).Scan(&revokedAt)

	if revokedAt != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
		return
	}

	accessToken, err := h.generateToken(claims.UserID, claims.TenantID, claims.Role,
		h.accessSecret, h.accessTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": accessToken,
		"expires_in":   int(h.accessTTL.Seconds()),
	})
}

// Logout revokes the user's refresh token
func (h *AuthHandler) Logout(c *gin.Context) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.ShouldBindJSON(&body)

	if body.RefreshToken != "" {
		tokenHash := hashToken(body.RefreshToken)
		_, _ = h.db.Pool().Exec(c.Request.Context(),
			"UPDATE refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1",
			tokenHash)
	}

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *AuthHandler) generateToken(userID, tenantID, role, secret string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := middleware.Claims{
		UserID:   userID,
		TenantID: tenantID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "h2o-gps",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
