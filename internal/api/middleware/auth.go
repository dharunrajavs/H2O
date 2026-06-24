package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   string `json:"uid"`
	TenantID string `json:"tid"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// JWTAuth validates the Bearer token and sets user context
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractBearerToken(c.GetHeader("Authorization"))
		if tokenStr == "" {
			// Also accept cookie
			tokenStr, _ = c.Cookie("access_token")
		}

		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("tenant_id", claims.TenantID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func extractBearerToken(header string) string {
	if strings.HasPrefix(header, "Bearer ") {
		return header[7:]
	}
	return ""
}

// RequireRole returns 403 if the authenticated user doesn't have the required role
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool)
	for _, r := range roles {
		allowed[r] = true
	}

	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if !allowed[role.(string)] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}
