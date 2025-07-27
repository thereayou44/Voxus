package server

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/thereayou/discord-lite/internal/handlers"
	"github.com/thereayou/discord-lite/internal/middleware"
)

func APIEndpoints(r *gin.Engine, s *Server) {
	// CORS configuration
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"http://localhost:3000", "http://localhost:5173"} // для разработки
	config.AllowCredentials = true
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	r.Use(cors.New(config))

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Auth endpoints
	auth := r.Group("/auth")
	{
		auth.POST("/register", s.AuthH.Register)
		auth.POST("/login", s.AuthH.Login)
		auth.POST("/logout", middleware.AuthMiddleware(s.JWTManager, s.Redis), s.AuthH.Logout)
	}

	// API endpoints с аутентификацией
	api := r.Group("/api/v1")
	api.Use(middleware.AuthMiddleware(s.JWTManager, s.Redis))
	{
		// User endpoints
		api.GET("/users/me", s.UserH.GetMe)
		api.PUT("/users/me", s.UserH.UpdateMe)
		api.GET("/users/:id", s.UserH.GetUser)
		api.GET("/users/search", s.UserH.SearchUsers)

		// Room endpoints
		api.POST("/rooms", s.RoomH.CreateRoom)
		api.GET("/rooms", s.RoomH.GetMyRooms)
		api.GET("/rooms/:id", s.RoomH.GetRoom)
		api.PUT("/rooms/:id", s.RoomH.UpdateRoom)
		api.DELETE("/rooms/:id", s.RoomH.DeleteRoom)
		api.POST("/rooms/:id/join", s.RoomH.JoinRoom)
		api.POST("/rooms/:id/leave", s.RoomH.LeaveRoom)
		api.GET("/rooms/:id/members", s.RoomH.GetRoomMembers)

		// Direct room
		api.POST("/rooms/direct", s.RoomH.CreateDirectRoom)

		// Message endpoints
		api.GET("/rooms/:id/messages", s.MessageH.GetRoomMessages)
		api.POST("/rooms/:id/messages", s.MessageH.SendMessage)
		api.PUT("/messages/:id", s.MessageH.UpdateMessage)
		api.DELETE("/messages/:id", s.MessageH.DeleteMessage)
	}

	// WebSocket endpoint с аутентификацией
	ws := r.Group("/ws")
	ws.Use(middleware.WSAuthMiddleware(s.JWTManager, s.Redis))
	{
		ws.GET("", s.WSHandler.HandleWebSocket)
	}
}
