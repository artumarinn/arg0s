// Package ipc es el protocolo JSON-RPC sobre Unix socket entre arg0s
// (CLI/TUI) y arg0sd (el motor), sección 18.2. Newline-delimited JSON
// en ambas direcciones sobre la misma conexión -- un Request de
// cliente puede recibir un Response inmediato Y, si es
// "events.subscribe", una serie de Event intercalados después, todo
// en la misma conexión (por eso el envelope Message: "kind" dice si
// esta línea es una respuesta o un evento).
//
// EventType son SIEMPRE explícitos y terminales -- esta es la tercera
// aparición de la carrera de streaming (Engram arg0s/lesson/stream-ctx-race,
// ver CLAUDE.md "RIESGO PRINCIPAL DE FASE 4 PARTE B"): un cliente
// nunca debe inferir "la task terminó bien" de que dejaron de llegar
// eventos o de que el socket se cerró. Todo Subscribe termina
// mandando EXACTAMENTE uno de done/failed/cancelled antes de dejar de
// escribir a la conexión.
package ipc

import "encoding/json"

// Request es lo que manda el cliente. ID lo elige el cliente y vuelve
// igual en el Response correspondiente -- permite tener varios
// requests en vuelo en la misma conexión (ej un task.cancel mientras
// hay un events.subscribe activo).
type Request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response responde UN Request por su ID.
type Response struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// EventType -- ver nota de paquete: siempre explícito, nunca inferido.
type EventType string

const (
	EventChunk     EventType = "chunk"
	EventDone      EventType = "done"
	EventFailed    EventType = "failed"
	EventCancelled EventType = "cancelled"
)

// Event es un mensaje de progreso de una task, sección 18.2
// ("events.subscribe ← stream de Event"). Seq es la posición de este
// evento en la secuencia de la task (0-indexado) -- permite a un
// cliente que se reconecta pedir "desde dónde" sin ambigüedad en vez
// de contar mensajes recibidos.
type Event struct {
	TaskID string    `json:"task_id"`
	Seq    int       `json:"seq"`
	Type   EventType `json:"type"`
	Delta  string    `json:"delta,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// Message es el envelope de wire format -- "kind" desambigua sin tener
// que adivinar por la forma del JSON.
type Message struct {
	Kind     string    `json:"kind"` // "response" | "event"
	Response *Response `json:"response,omitempty"`
	Event    *Event    `json:"event,omitempty"`
}

// Métodos soportados (subset de sección 18.2 -- el resto llega cuando
// la TUI los necesite de verdad, no antes).
const (
	MethodTaskSubmit      = "task.submit"
	MethodTaskCancel      = "task.cancel"
	MethodEventsSubscribe = "events.subscribe"
	MethodDaemonPing      = "daemon.ping"
	MethodDaemonShutdown  = "daemon.shutdown"
)

type SubmitParams struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model,omitempty"`
}

type SubmitResult struct {
	TaskID string `json:"task_id"`
}

type CancelParams struct {
	TaskID string `json:"task_id"`
}

// SubscribeParams.From permite reenganchar desde un punto conocido --
// 0 (default) trae el historial completo desde el principio, sección
// 18.1 "podés cerrar y reabrir la TUI sin perder una ejecución larga".
type SubscribeParams struct {
	TaskID string `json:"task_id"`
	From   int    `json:"from,omitempty"`
}
