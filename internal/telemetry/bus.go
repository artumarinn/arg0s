package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Bus es un event bus en memoria con sink opcional a SQLite (tabla
// `events`, sección 19). Los suscriptores reciben eventos por canal;
// nadie bloquea a nadie — un suscriptor lento se salta eventos en vez
// de frenar al publisher.
type Bus struct {
	mu          sync.RWMutex
	subscribers []chan Event
	db          *sql.DB
}

func NewBus(db *sql.DB) *Bus {
	return &Bus{db: db}
}

// Subscribe devuelve un canal que recibe todo evento publicado después
// de esta llamada.
func (b *Bus) Subscribe() <-chan Event {
	ch := make(chan Event, 32)
	b.mu.Lock()
	b.subscribers = append(b.subscribers, ch)
	b.mu.Unlock()
	return ch
}

// Publish emite un evento a todos los suscriptores y lo persiste en
// SQLite si el bus tiene un *sql.DB configurado.
func (b *Bus) Publish(ctx context.Context, ev Event) error {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}

	b.mu.RLock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default: // suscriptor lento, se salta el evento
		}
	}
	b.mu.RUnlock()

	if b.db == nil {
		return nil
	}
	return b.persist(ctx, ev)
}

func (b *Bus) persist(ctx context.Context, ev Event) error {
	payload, err := json.Marshal(ev.Payload)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}

	var durationMs any
	if ev.Duration != nil {
		durationMs = ev.Duration.Milliseconds()
	}

	_, err = b.db.ExecContext(ctx, `
		INSERT INTO events (id, session_id, run_id, task_id, type, payload, duration_ms, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, nullIfEmpty(ev.SessionID), nullIfEmpty(ev.RunID), nullIfEmpty(ev.TaskID),
		string(ev.Type), string(payload), durationMs, ev.Timestamp.Unix(),
	)
	if err != nil {
		return fmt.Errorf("persist event %s: %w", ev.Type, err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
