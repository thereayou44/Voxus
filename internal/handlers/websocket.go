package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"net/http"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
		// TODO: check origin
	},
}

func WebSocketHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Простой echo сервер для начала
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if err := conn.WriteMessage(messageType, p); err != nil {
			break
		}
	}
}
