package ipc

import (
	"context"
	"sync"
)

// task es el estado en memoria de una corrida enviada por task.submit.
// Guarda TODOS los eventos emitidos (para reenganche, sección 18.1) y
// expone Subscribe como un broadcast: cualquier cantidad de llamadas a
// Subscribe (secuenciales o -- si el daemon corriera para siempre --
// concurrentes) pueden pedir "desde el evento N" y reciben el
// historial más lo que vaya llegando, hasta el evento terminal.
type task struct {
	cancel context.CancelFunc

	mu     sync.Mutex
	events []Event
	done   bool          // true una vez que se agregó un evento terminal (done/failed/cancelled)
	notify chan struct{} // se cierra y se reemplaza en cada append -- patrón broadcast
}

func newTask(cancel context.CancelFunc) *task {
	return &task{cancel: cancel, notify: make(chan struct{})}
}

// append agrega un evento a la secuencia. Un evento terminal marca
// done=true -- Subscribe no vuelve a bloquear después de mandarlo.
func (t *task) append(e Event) {
	t.mu.Lock()
	e.Seq = len(t.events)
	t.events = append(t.events, e)
	if e.Type != EventChunk {
		t.done = true
	}
	old := t.notify
	t.notify = make(chan struct{})
	t.mu.Unlock()
	close(old)
}

// Subscribe manda a send() todo evento desde from en adelante --
// primero el historial ya guardado, después lo que vaya llegando --
// hasta que se manda un evento terminal o ctx se cancela (el cliente
// se desconectó). Nunca vuelve sin haber mandado el evento terminal,
// salvo que ctx se cancele antes -- esa es la garantía de "nunca
// inferir done del silencio" en el lado del productor.
func (t *task) Subscribe(ctx context.Context, from int, send func(Event) error) error {
	for {
		t.mu.Lock()
		var pending []Event
		if from < len(t.events) {
			pending = append(pending, t.events[from:]...)
		}
		notify := t.notify
		done := t.done
		total := len(t.events)
		t.mu.Unlock()

		for _, e := range pending {
			if err := send(e); err != nil {
				return err
			}
			from++
		}
		if done && from >= total {
			return nil
		}

		select {
		case <-notify:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (t *task) Cancel() {
	t.cancel()
}
