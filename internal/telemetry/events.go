// Package telemetry implementa el event bus de Arg0s (sección 16.1).
package telemetry

import "time"

type EventType string

const (
	EventSessionStarted EventType = "SessionStarted"
	EventSessionEnded   EventType = "SessionEnded"
	EventTaskCreated    EventType = "TaskCreated"
	EventTaskCompleted  EventType = "TaskCompleted"
	EventTaskFailed     EventType = "TaskFailed"
	EventTaskCancelled  EventType = "TaskCancelled"

	// Emitidos por internal/execution (Fase 1) — ver sección 16.1.
	EventModelStarted   EventType = "ModelStarted"
	EventModelCompleted EventType = "ModelCompleted"
	EventModelFailed    EventType = "ModelFailed"
	EventModelRetried   EventType = "ModelRetried"
	EventModelFellBack  EventType = "ModelFellBack"
)

// Event es la unidad de la observabilidad de Arg0s (sección 16.1).
type Event struct {
	ID        string
	Type      EventType
	SessionID string
	RunID     string
	TaskID    string
	Timestamp time.Time
	Payload   map[string]any
	Duration  *time.Duration
}
