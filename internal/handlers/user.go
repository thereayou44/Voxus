package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/thereayou/discord-lite/internal/database"
	"github.com/thereayou/discord-lite/internal/middleware"
	"net/http"
)

type UserHandler struct {
	db *database.Database
}

func NewUserHandler(db *database.Database) *UserHandler {
	return &UserHandler{db: db}
}

// GetMe возвращает информацию о текущем пользователе
func (h *UserHandler) GetMe(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

	user, err := h.db.GetUser(userID.String())
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"username":     user.Username,
		"email":        user.Email,
		"avatar_url":   user.AvatarURL,
		"created_at":   user.CreatedAt,
		"last_seen_at": user.LastSeenAt,
	})
}

// UpdateMe обновляет информацию текущего пользователя
func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID := c.MustGet(middleware.UserIDKey).(uuid.UUID)

	var req struct {
		Username  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.db.GetUser(userID.String())
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Обновляем только переданные поля
	if req.Username != "" {
		user.Username = req.Username
	}
	if req.AvatarURL != "" {
		user.AvatarURL = req.AvatarURL
	}

	if err := h.db.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         user.ID,
		"username":   user.Username,
		"email":      user.Email,
		"avatar_url": user.AvatarURL,
	})
}

// GetUser возвращает информацию о пользователе по ID
func (h *UserHandler) GetUser(c *gin.Context) {
	userID := c.Param("id")

	user, err := h.db.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           user.ID,
		"username":     user.Username,
		"avatar_url":   user.AvatarURL,
		"last_seen_at": user.LastSeenAt,
	})
}

// SearchUsers поиск пользователей по username
func (h *UserHandler) SearchUsers(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter is required"})
		return
	}

	users, err := h.db.SearchUsersByUsername(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search users"})
		return
	}

	// Форматируем ответ
	result := make([]gin.H, len(users))
	for i, user := range users {
		result[i] = gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"avatar_url": user.AvatarURL,
		}
	}

	c.JSON(http.StatusOK, gin.H{"users": result})
}
