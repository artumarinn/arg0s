// Package telemetry implementa el event bus de Arg0s (sección 16.1).
// En Fase 0 solo existen los tipos de evento y la infraestructura de
// bus + persistencia — nada todavía emite eventos de ejecución real
// (eso llega con execution/orchestrator en fases posteriores).
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
