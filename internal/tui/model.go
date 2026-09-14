// Package tui es SOLO presentación (P1): consume ipc.Event del daemon
// y renderiza. Ninguna decisión de routing/fusion/policy vive acá --
// esas ya se tomaron en arg0sd antes de que el primer Event llegue.
//
// Alcance de Fase 4 parte B punto 2: el daemon hoy corre generación
// directa (un modelo, sin pasar por router/fusion todavía -- ver
// cmd/arg0sd/main.go), así que solo hay un bloque real que mostrar:
// GENERATION. Render progresivo (sección 18.4) se cumple trivialmente
// con un solo bloque: arranca "pending", pasa a "running" en el primer
// chunk. Cuando el daemon empiece a emitir fases de router/fusion,
// este Model gana más bloques -- no se inventan bloques vacíos ahora
// para "parecerse" al mockup de la sección 18.3.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/artumarinn/arg0s/internal/ipc"
)

// BlockState -- sección 18.4: pending/running/done/failed/skipped
// (acá "skipped" no aplica todavía, ver nota de paquete; "cancelled" es
// el estado real que sí puede pasar).
type BlockState string

const (
	StatePending   BlockState = "pending"
	StateRunning   BlockState = "running"
	StateDone      BlockState = "done"
	StateFailed    BlockState = "failed"
	StateCancelled BlockState = "cancelled"
)

// connErrMsg es un error de LA CONEXIÓN IPC (perdimos al daemon) --
// distinto de que la task haya fallado, no hay que confundirlos en la
// UI.
type connErrMsg struct{ err error }

// canceller es lo mínimo que el Model necesita del *ipc.Client -- una
// interfaz chica para poder testear Update() sin un daemon real
// (mismo criterio que fusion.Synthesizer/contextc.Summarizer).
type canceller interface {
	Cancel(ctx context.Context, taskID string) error
}

type Model struct {
	client canceller
	taskID string

	state      BlockState
	content    strings.Builder
	errMsg     string
	spinner    spinner.Model
	cancelling bool
}

func New(client canceller, taskID string) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	return Model{client: client, taskID: taskID, state: StatePending, spinner: sp}
}

func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// ctrl+c cancela la task, no mata la TUI (DoD de Fase 4) --
			// solo si todavía está viva; si ya terminó, no hay nada que
			// cancelar y ctrl+c actúa como salir.
			if m.state == StatePending || m.state == StateRunning {
				m.cancelling = true
				return m, m.cancelCmd()
			}
			return m, tea.Quit
		case "q":
			// Cerrar la TUI -- la task sigue viva en el daemon (proceso
			// separado). Reabrir es `arg0s tui attach <task_id>`.
			return m, tea.Quit
		}

	case ipc.Event:
		switch msg.Type {
		case ipc.EventChunk:
			m.state = StateRunning
			m.content.WriteString(msg.Delta)
		case ipc.EventDone:
			m.state = StateDone
		case ipc.EventFailed:
			m.state = StateFailed
			m.errMsg = msg.Error
		case ipc.EventCancelled:
			m.state = StateCancelled
		}
		return m, nil

	case connErrMsg:
		m.errMsg = msg.err.Error()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) cancelCmd() tea.Cmd {
	client, taskID := m.client, m.taskID
	return func() tea.Msg {
		if err := client.Cancel(context.Background(), taskID); err != nil {
			return connErrMsg{err}
		}
		return nil
	}
}

func (m Model) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ARG0S -- task %s\n\n", m.taskID)
	b.WriteString(m.blockHeader())
	b.WriteString("\n")
	b.WriteString(m.content.String())
	if m.content.Len() > 0 {
		b.WriteString("\n")
	}
	if m.errMsg != "" {
		fmt.Fprintf(&b, "\nerror: %s\n", m.errMsg)
	}
	if m.cancelling && m.state != StateCancelled {
		b.WriteString("\ncancelando...\n")
	}
	b.WriteString("\nctrl+c cancelar · q cerrar (la task sigue en el daemon)\n")
	return b.String()
}

func (m Model) blockHeader() string {
	switch m.state {
	case StatePending:
		return "[ GENERATION ] pending"
	case StateRunning:
		return fmt.Sprintf("[ GENERATION ] %s running", m.spinner.View())
	case StateDone:
		return "[ GENERATION ] ✓ done"
	case StateFailed:
		return "[ GENERATION ] ✗ failed"
	case StateCancelled:
		return "[ GENERATION ] ⊘ cancelled"
	default:
		return ""
	}
}
