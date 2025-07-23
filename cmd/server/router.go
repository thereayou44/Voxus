package server

import (
	"github.com/gin-gonic/gin"
	"github.com/thereayou/discord-lite/internal/handlers"
)

func APIEndpoints(r *gin.Engine, authH *handlers.AuthHandler) {
	// Auth endpoints
	auth := r.Group("/auth")
	{
		auth.POST("/register", authH.Register)
		auth.POST("/login", authH.Login)
	}

	//// API endpoints
	//api := r.Group("/api/v1")
	//{
	//}
}
