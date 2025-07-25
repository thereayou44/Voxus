package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/thereayou/discord-lite/pkg/auth"
)

const UserIDKey = "userID"

// AuthMiddleware проверяет JWT токен
func AuthMiddleware(jwtManager *auth.JWTManager, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := auth.ExtractTokenFromHeader(c.Request)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid token"})
			c.Abort()
			return
		}

		// Проверяем, не в черном списке ли токен
		exists, err := redisClient.Exists(context.Background(), "blacklist:"+token).Result()
		if err != nil || exists > 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token is blacklisted"})
			c.Abort()
			return
		}

		claims, err := jwtManager.Verify(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
			c.Abort()
			return
		}

		c.Set(UserIDKey, userID)
		c.Next()
	}
}

// WSAuthMiddleware специальный middleware для WebSocket
func WSAuthMiddleware(jwtManager *auth.JWTManager, redisClient *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
					token = parts[1]
				}
			}
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}

		// Проверяем черный список
		exists, err := redisClient.Exists(context.Background(), "blacklist:"+token).Result()
		if err != nil || exists > 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token is blacklisted"})
			c.Abort()
			return
		}

		claims, err := jwtManager.Verify(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
			c.Abort()
			return
		}

		c.Set(UserIDKey, userID)
		c.Next()
	}
}
