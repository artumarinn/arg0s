package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artumarinn/arg0s/internal/ipc"
)

type fakeCanceller struct {
	calls    int
	lastTask string
	err      error
}

func (f *fakeCanceller) Cancel(ctx context.Context, taskID string) error {
	f.calls++
	f.lastTask = taskID
	return f.err
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestModel_StartsPending(t *testing.T) {
	m := New(&fakeCanceller{}, "tsk_1")
	if m.state != StatePending {
		t.Fatalf("state = %q, want pending", m.state)
	}
	if !strings.Contains(m.View(), "pending") {
		t.Fatalf("View() no muestra pending: %s", m.View())
	}
}

func TestModel_ChunkEvent_MovesToRunningAndAppendsContent(t *testing.T) {
	m := New(&fakeCanceller{}, "tsk_1")
	m = update(t, m, ipc.Event{Type: ipc.EventChunk, Delta: "hola "})
	m = update(t, m, ipc.Event{Type: ipc.EventChunk, Delta: "mundo"})

	if m.state != StateRunning {
		t.Fatalf("state = %q, want running", m.state)
	}
	if m.content.String() != "hola mundo" {
		t.Fatalf("content = %q, want %q", m.content.String(), "hola mundo")
	}
	if !strings.Contains(m.View(), "hola mundo") {
		t.Fatalf("View() no muestra el contenido acumulado: %s", m.View())
	}
}

func TestModel_DoneEvent_MovesToDoneExplicitly(t *testing.T) {
	m := New(&fakeCanceller{}, "tsk_1")
	m = update(t, m, ipc.Event{Type: ipc.EventChunk, Delta: "x"})
	m = update(t, m, ipc.Event{Type: ipc.EventDone})

	if m.state != StateDone {
		t.Fatalf("state = %q, want done", m.state)
	}
	if !strings.Contains(m.View(), "✓ done") {
		t.Fatalf("View() no muestra done: %s", m.View())
	}
}

func TestModel_FailedEvent_ShowsError(t *testing.T) {
	m := New(&fakeCanceller{}, "tsk_1")
	m = update(t, m, ipc.Event{Type: ipc.EventFailed, Error: "boom"})

	if m.state != StateFailed {
		t.Fatalf("state = %q, want failed", m.state)
	}
	if !strings.Contains(m.View(), "boom") {
		t.Fatalf("View() no muestra el error: %s", m.View())
	}
}

func TestModel_CtrlC_WhileRunning_CancelsWithoutQuitting(t *testing.T) {
	fake := &fakeCanceller{}
	m := New(fake, "tsk_1")
	m = update(t, m, ipc.Event{Type: ipc.EventChunk, Delta: "x"})

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("esperaba un tea.Cmd (el cancel), got nil")
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); isQuit {
		t.Fatal("ctrl+c mientras corre NO debe mandar tea.Quit -- cancela, no mata la TUI")
	}
	if fake.calls != 1 || fake.lastTask != "tsk_1" {
		t.Fatalf("esperaba 1 llamada a Cancel(tsk_1), got calls=%d task=%q", fake.calls, fake.lastTask)
	}
}

func TestModel_CtrlC_AfterDone_Quits(t *testing.T) {
	fake := &fakeCanceller{}
	m := New(fake, "tsk_1")
	m = update(t, m, ipc.Event{Type: ipc.EventDone})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("esperaba tea.Quit, got nil cmd")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("ctrl+c después de terminado debería salir (nada que cancelar)")
	}
	if fake.calls != 0 {
		t.Fatalf("no debería haber llamado a Cancel, calls=%d", fake.calls)
	}
}

func TestModel_Q_AlwaysQuitsWithoutCancelling(t *testing.T) {
	fake := &fakeCanceller{}
	m := New(fake, "tsk_1")
	m = update(t, m, ipc.Event{Type: ipc.EventChunk, Delta: "x"})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("esperaba tea.Quit")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("'q' debería salir de la TUI")
	}
	if fake.calls != 0 {
		t.Fatal("'q' no debe cancelar la task -- solo cierra la TUI, DoD de Fase 4")
	}
}

// TestModel_ReplayFromZero_RebuildsFullState simula lo que pasa al
// reenganchar (arg0s tui attach): Subscribe(from=0) manda TODO el
// historial de nuevo, y el Model tiene que terminar en el mismo estado
// que si lo hubiera visto en vivo.
func TestModel_ReplayFromZero_RebuildsFullState(t *testing.T) {
	m := New(&fakeCanceller{}, "tsk_1")
	history := []ipc.Event{
		{Type: ipc.EventChunk, Delta: "hola "},
		{Type: ipc.EventChunk, Delta: "mundo"},
		{Type: ipc.EventDone},
	}
	for _, e := range history {
		m = update(t, m, e)
	}
	if m.state != StateDone || m.content.String() != "hola mundo" {
		t.Fatalf("replay no reconstruyó el estado: state=%q content=%q", m.state, m.content.String())
	}
}
