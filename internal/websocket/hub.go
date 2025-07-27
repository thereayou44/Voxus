package websocket

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// MessageType определяет типы сообщений
type MessageType string

const (
	// Системные типы
	TypeConnect    MessageType = "connect"
	TypeDisconnect MessageType = "disconnect"
	TypePing       MessageType = "ping"
	TypePong       MessageType = "pong"

	// Типы сообщений
	TypeMessage       MessageType = "message"
	TypeMessageEdit   MessageType = "message_edit"
	TypeMessageDelete MessageType = "message_delete"

	// Типы комнат
	TypeRoomJoin  MessageType = "room_join"
	TypeRoomLeave MessageType = "room_leave"
	TypeRoomUsers MessageType = "room_users"

	// Типы статусов
	TypeUserStatus  MessageType = "user_status"
	TypeUserOnline  MessageType = "user_online"
	TypeUserOffline MessageType = "user_offline"
)

type Message struct {
	Type      MessageType     `json:"type"`
	RoomID    *uuid.UUID      `json:"room_id,omitempty"`
	UserID    uuid.UUID       `json:"user_id"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

type Client struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Conn   *websocket.Conn
	Send   chan []byte
	Rooms  map[uuid.UUID]bool
	Hub    *Hub
	mu     sync.RWMutex
}

type Hub struct {
	clients map[uuid.UUID]*Client

	// Клиенты по UserID (один пользователь может иметь несколько соединений)
	userClients map[uuid.UUID]map[uuid.UUID]*Client

	// Клиенты в комнатах
	rooms map[uuid.UUID]map[uuid.UUID]*Client

	// Каналы для регистрации/отмены регистрации
	register   chan *Client
	unregister chan *Client

	broadcast chan *BroadcastMessage

	mu sync.RWMutex

	// Контекст для graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

type BroadcastMessage struct {
	RoomID  *uuid.UUID
	UserID  *uuid.UUID // nil = всем в комнате
	Message []byte
	Exclude *uuid.UUID
}

// NewHub создает новый Hub
func NewHub() *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{
		clients:     make(map[uuid.UUID]*Client),
		userClients: make(map[uuid.UUID]map[uuid.UUID]*Client),
		rooms:       make(map[uuid.UUID]map[uuid.UUID]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan *BroadcastMessage),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Run запускает hub
func (h *Hub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return

		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)

		case <-ticker.C:
			h.ping()
		}
	}
}

// Stop останавливает hub
func (h *Hub) Stop() {
	h.cancel()

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, client := range h.clients {
		close(client.Send)
		client.Conn.Close()
	}
}

// Register регистрирует нового клиента
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister отменяет регистрацию клиента
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client.ID] = client

	if _, ok := h.userClients[client.UserID]; !ok {
		h.userClients[client.UserID] = make(map[uuid.UUID]*Client)
	}
	h.userClients[client.UserID][client.ID] = client

	log.Printf("Client registered: %s (User: %s)", client.ID, client.UserID)

	// Отправляем уведомление о подключении пользователя
	h.notifyUserStatus(client.UserID, TypeUserOnline)
}

func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client.ID]; ok {
		// Удаляем из всех комнат
		for roomID := range client.Rooms {
			h.removeFromRoomUnsafe(client, roomID)
		}

		// Удаляем из списка клиентов пользователя
		if userClients, ok := h.userClients[client.UserID]; ok {
			delete(userClients, client.ID)
			if len(userClients) == 0 {
				delete(h.userClients, client.UserID)
				// Отправляем уведомление об отключении пользователя
				h.notifyUserStatus(client.UserID, TypeUserOffline)
			}
		}

		delete(h.clients, client.ID)
		close(client.Send)

		log.Printf("Client unregistered: %s (User: %s)", client.ID, client.UserID)
	}
}

// JoinRoom добавляет клиента в комнату
func (h *Hub) JoinRoom(client *Client, roomID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.rooms[roomID]; !ok {
		h.rooms[roomID] = make(map[uuid.UUID]*Client)
	}

	h.rooms[roomID][client.ID] = client
	client.mu.Lock()
	client.Rooms[roomID] = true
	client.mu.Unlock()

	// Уведомляем других участников о присоединении
	joinMsg := Message{
		Type:      TypeRoomJoin,
		RoomID:    &roomID,
		UserID:    client.UserID,
		Timestamp: time.Now(),
	}

	if data, err := json.Marshal(joinMsg); err == nil {
		h.broadcastToRoomExcept(roomID, data, client.ID)
	}

	// Отправляем список участников новому клиенту
	h.sendRoomUsers(client, roomID)
}

// LeaveRoom удаляет клиента из комнаты
func (h *Hub) LeaveRoom(client *Client, roomID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.removeFromRoomUnsafe(client, roomID)
}

func (h *Hub) removeFromRoomUnsafe(client *Client, roomID uuid.UUID) {
	if room, ok := h.rooms[roomID]; ok {
		if _, ok := room[client.ID]; ok {
			delete(room, client.ID)
			client.mu.Lock()
			delete(client.Rooms, roomID)
			client.mu.Unlock()

			if len(room) == 0 {
				delete(h.rooms, roomID)
			} else {
				// Уведомляем других участников
				leaveMsg := Message{
					Type:      TypeRoomLeave,
					RoomID:    &roomID,
					UserID:    client.UserID,
					Timestamp: time.Now(),
				}

				if data, err := json.Marshal(leaveMsg); err == nil {
					h.broadcastToRoomExcept(roomID, data, client.ID)
				}
			}
		}
	}
}

// SendToUser отправляет сообщение пользователю
func (h *Hub) SendToUser(userID uuid.UUID, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.userClients[userID]; ok {
		for _, client := range clients {
			select {
			case client.Send <- message:
			default:
				log.Printf("Client %s send channel full", client.ID)
			}
		}
	}
}

// SendToRoom отправляет сообщение в комнату
func (h *Hub) SendToRoom(roomID uuid.UUID, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	h.broadcastToRoomExcept(roomID, message, uuid.Nil)
}

func (h *Hub) broadcastMessage(bm *BroadcastMessage) {
	if bm.RoomID != nil {
		h.SendToRoom(*bm.RoomID, bm.Message)
	} else if bm.UserID != nil {
		h.SendToUser(*bm.UserID, bm.Message)
	}
}

func (h *Hub) broadcastToRoomExcept(roomID uuid.UUID, message []byte, excludeID uuid.UUID) {
	if room, ok := h.rooms[roomID]; ok {
		for _, client := range room {
			if client.ID != excludeID {
				select {
				case client.Send <- message:
				default:
					log.Printf("Client %s send channel full", client.ID)
				}
			}
		}
	}
}

func (h *Hub) sendRoomUsers(client *Client, roomID uuid.UUID) {
	users := make([]uuid.UUID, 0)

	if room, ok := h.rooms[roomID]; ok {
		userMap := make(map[uuid.UUID]bool)
		for _, c := range room {
			userMap[c.UserID] = true
		}

		for userID := range userMap {
			users = append(users, userID)
		}
	}

	msg := Message{
		Type:      TypeRoomUsers,
		RoomID:    &roomID,
		UserID:    client.UserID,
		Timestamp: time.Now(),
	}

	if data, err := json.Marshal(users); err == nil {
		msg.Data = data
		if msgData, err := json.Marshal(msg); err == nil {
			select {
			case client.Send <- msgData:
			default:
				log.Printf("Failed to send room users to client %s", client.ID)
			}
		}
	}
}

// notifyUserStatus уведомляет о статусе пользователя
func (h *Hub) notifyUserStatus(userID uuid.UUID, status MessageType) {
	msg := Message{
		Type:      status,
		UserID:    userID,
		Timestamp: time.Now(),
	}

	if data, err := json.Marshal(msg); err == nil {
		// TODO: отправлять только друзьям или контактам
		for _, client := range h.clients {
			select {
			case client.Send <- data:
			default:
			}
		}
	}
}

func (h *Hub) ping() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	msg := Message{
		Type:      TypePing,
		Timestamp: time.Now(),
	}

	if data, err := json.Marshal(msg); err == nil {
		for _, client := range h.clients {
			select {
			case client.Send <- data:
			default:
			}
		}
	}
}

// GetOnlineUsers возвращает список онлайн пользователей
func (h *Hub) GetOnlineUsers() []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()

	users := make([]uuid.UUID, 0, len(h.userClients))
	for userID := range h.userClients {
		users = append(users, userID)
	}
	return users
}

// GetRoomUsers возвращает список пользователей в комнате
func (h *Hub) GetRoomUsers(roomID uuid.UUID) []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()

	userMap := make(map[uuid.UUID]bool)
	if room, ok := h.rooms[roomID]; ok {
		for _, client := range room {
			userMap[client.UserID] = true
		}
	}

	users := make([]uuid.UUID, 0, len(userMap))
	for userID := range userMap {
		users = append(users, userID)
	}
	return users
}
