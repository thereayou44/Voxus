package websocket

import "errors"

var (
	ErrClientQueueFull = errors.New("client message queue is full")
	ErrInvalidMessage  = errors.New("invalid message format")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrRoomNotFound    = errors.New("room not found")
	ErrUserNotInRoom   = errors.New("user not in room")
)
