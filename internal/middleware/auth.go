package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"colink-server/internal/pkg"
)

const ContextUserIDKey = "userId"

type AuthMiddleware struct {
	secret string
}

func NewAuthMiddleware(secret string) *AuthMiddleware {
	return &AuthMiddleware{secret: secret}
}

func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    pkg.CodeUnauthorized,
				"data":    nil,
				"message": "unauthorized",
			})
			c.Abort()
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := pkg.ParseAccessToken(m.secret, token)
		if err != nil || claims.UserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    pkg.CodeUnauthorized,
				"data":    nil,
				"message": "unauthorized",
			})
			c.Abort()
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Next()
	}
}
