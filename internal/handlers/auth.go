package handlers

import (
	"context"
	"gorm.io/gorm"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"golang.org/x/crypto/bcrypt"

	"github.com/thereayou/discord-lite/internal/database"
	"github.com/thereayou/discord-lite/internal/handlers/dto"
	"github.com/thereayou/discord-lite/internal/models"
	"github.com/thereayou/discord-lite/pkg/auth"
)

type AuthHandler struct {
	db         *database.Database
	jwtManager *auth.JWTManager
	redis      *redis.Client
}

func NewAuthHandler(db *database.Database, jwtMgr *auth.JWTManager, rdb *redis.Client) *AuthHandler {
	return &AuthHandler{db: db, jwtManager: jwtMgr, redis: rdb}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot hash password"})
		return
	}

	user := &models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}

	if err := h.db.SaveUser(user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "user registered"})
}

// Login выдаёт JWT и обновляет last_seen
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.db.FindUserByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := h.db.UpdateLastSeen(user.ID.String()); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update last seen"})
		return
	}

	token, err := h.jwtManager.Generate(user.ID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// Logout ставит токен в черный список в Redis до истечения
func (h *AuthHandler) Logout(c *gin.Context) {
	rawToken, err := auth.ExtractTokenFromHeader(c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	exp, err := h.jwtManager.Expiry(rawToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	ttl := time.Until(exp)
	h.redis.Set(context.Background(), "blacklist:"+rawToken, 1, ttl)

	c.Status(http.StatusOK)
}
