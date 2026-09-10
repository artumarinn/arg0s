package core

import "time"

// Message es una entrada del historial de conversación de una Task.
type Message struct {
	Role      string // system | user | assistant | tool
	Content   string
	CreatedAt time.Time
}
