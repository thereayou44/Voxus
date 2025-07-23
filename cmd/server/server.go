package server

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"github.com/thereayou/discord-lite/internal/database"
	"github.com/thereayou/discord-lite/internal/handlers"
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
	AuthH      *handlers.AuthHandler
}

func NewServer() *Server {
	if err := godotenv.Load(".env.local"); err != nil {
		if err := godotenv.Load(); err != nil {
			log.Println(".env not found, using environment variables")
		}
	}

	dbConn := &database.Database{}
	if err := dbConn.Connect(); err != nil {
		log.Fatalf("Postgres connect failed: %v", err)
	}

	redisOpts, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	if err != nil {
		log.Fatalf("invalid REDIS_URL: %v", err)
	}
	rdb := redis.NewClient(redisOpts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis connect failed: %v", err)
	}

	jwtMgr := auth.NewJWTManager(
		os.Getenv("JWT_SECRET"),
		24*time.Hour,
	)

	authH := handlers.NewAuthHandler(dbConn, jwtMgr, rdb)

	router := gin.Default()
	APIEndpoints(router, authH)

	return &Server{
		Router:     router,
		DB:         dbConn,
		Redis:      rdb,
		JWTManager: jwtMgr,
		AuthH:      authH,
	}
}

func (s *Server) Run() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on port %s", port)
	if err := s.Router.Run(":" + port); err != nil {
		log.Fatalf("Server run error: %v", err)
	}
}
