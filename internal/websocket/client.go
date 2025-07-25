package websocket

import (
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// Время ожидания записи
	writeWait = 10 * time.Second

	// Время ожидания pong от клиента
	pongWait = 60 * time.Second

	// Интервал отправки ping
	pingPeriod = (pongWait * 9) / 10

	// Максимальный размер сообщения
	maxMessageSize = 512 * 1024 // 512KB
)

type ClientMessageHandler interface {
	HandleMessage(client *Client, msg *Message) error
}

func NewClient(hub *Hub, conn *websocket.Conn, userID uuid.UUID) *Client {
	return &Client{
		ID:     uuid.New(),
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		Rooms:  make(map[uuid.UUID]bool),
		Hub:    hub,
	}
}

// ReadPump читает сообщения от клиента
func (c *Client) ReadPump(handler ClientMessageHandler) {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		var msg Message
		err := c.Conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		msg.UserID = c.UserID

		switch msg.Type {
		case TypePong:
			continue

		case TypeRoomJoin:
			if msg.RoomID != nil {
				c.Hub.JoinRoom(c, *msg.RoomID)
			}
			continue

		case TypeRoomLeave:
			if msg.RoomID != nil {
				c.Hub.LeaveRoom(c, *msg.RoomID)
			}
			continue
		}

		if handler != nil {
			if err := handler.HandleMessage(c, &msg); err != nil {
				log.Printf("Error handling message: %v", err)
				c.SendError(err.Error())
			}
		}
	}
}

// WritePump отправляет сообщения клиенту
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub закрыл канал
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.Conn.WriteMessage(websocket.TextMessage, message)

			// Отправляем все накопившиеся сообщения
			n := len(c.Send)
			for i := 0; i < n; i++ {
				<-c.Send
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) SendMessage(msgType MessageType, data interface{}) error {
	msg := Message{
		Type:      msgType,
		Timestamp: time.Now(),
	}

	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return err
		}
		msg.Data = jsonData
	}

	msgData, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	select {
	case c.Send <- msgData:
		return nil
	default:
		return ErrClientQueueFull
	}
}

func (c *Client) SendError(errorMsg string) {
	c.SendMessage("error", map[string]string{
		"error": errorMsg,
	})
}

func (c *Client) IsInRoom(roomID uuid.UUID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Rooms[roomID]
}

func (c *Client) GetRooms() []uuid.UUID {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rooms := make([]uuid.UUID, 0, len(c.Rooms))
	for roomID := range c.Rooms {
		rooms = append(rooms, roomID)
	}
	return rooms
}
