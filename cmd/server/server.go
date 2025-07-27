package server

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"github.com/thereayou/discord-lite/internal/database"
	"github.com/thereayou/discord-lite/internal/handlers"
	"github.com/thereayou/discord-lite/internal/websocket"
	"github.com/thereayou/discord-lite/pkg/auth"
	"log"
	"os"
	"time"
)

type Server struct {
	Router     *gin.Engine
	DB         *database.Database
	Redis      *redis.Client
	JWTManager *auth.JWTManager
	Hub        *websocket.Hub
	// Handlers
	AuthH        *handlers.AuthHandler
	UserH        *handlers.UserHandler
	RoomH        *handlers.RoomHandler
	HTTPMessageH *handlers.HTTPMessageHandler
	WSHandler    *handlers.WebSocketHandler
}

func NewServer() *Server {
	// Load environment variables
	if err := godotenv.Load(".env.local"); err != nil {
		if err := godotenv.Load(); err != nil {
			log.Println(".env not found, using environment variables")
		}
	}

	// Database connection
	dbConn := &database.Database{}
	if err := dbConn.Connect(); err != nil {
		log.Fatalf("Postgres connect failed: %v", err)
	}

	// Redis connection
	redisOpts := &redis.Options{
		Addr:     os.Getenv("REDIS_URL"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	}

	// Если REDIS_URL содержит полный URL, парсим его
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" && redisURL != "localhost:6379" {
		opts, err := redis.ParseURL(redisURL)
		if err == nil {
			redisOpts = opts
		}
	}

	rdb := redis.NewClient(redisOpts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis connect failed: %v", err)
	}

	// JWT Manager
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "your-secret-key-change-this-in-production"
		log.Println("WARNING: Using default JWT secret. Change this in production!")
	}

	jwtMgr := auth.NewJWTManager(jwtSecret, 24*time.Hour)

	// WebSocket Hub
	hub := websocket.NewHub()
	go hub.Run()

	// Initialize handlers
	authH := handlers.NewAuthHandler(dbConn, jwtMgr, rdb)
	userH := handlers.NewUserHandler(dbConn)
	roomH := handlers.NewRoomHandler(dbConn, hub)

	// Message handler нужен для WebSocket handler
	msgHandler := handlers.NewMessageHandler(dbConn, hub)
	wsHandler := handlers.NewWebSocketHandler(hub, msgHandler)

	// HTTP message handler для REST API
	messageH := handlers.NewHTTPMessageHandler(dbConn)

	// Setup router
	router := gin.Default()

	// Set trusted proxies
	router.SetTrustedProxies(nil)

	server := &Server{
		Router:       router,
		DB:           dbConn,
		Redis:        rdb,
		JWTManager:   jwtMgr,
		Hub:          hub,
		AuthH:        authH,
		UserH:        userH,
		RoomH:        roomH,
		HTTPMessageH: messageH,
		WSHandler:    wsHandler,
	}

	// Setup routes
	APIEndpoints(router, server)

	return server
}

func (s *Server) Run() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	log.Printf("WebSocket hub is running")

	if err := s.Router.Run(":" + port); err != nil {
		log.Fatalf("Server run error: %v", err)
	}
}

func (s *Server) Shutdown() {
	log.Println("Shutting down server...")
	s.Hub.Stop()
	// Закрываем соединения
	if s.Redis != nil {
		s.Redis.Close()
	}
}
