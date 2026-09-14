package main

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/ipc"
	"github.com/artumarinn/arg0s/internal/tui"
)

// newTUICmd es el punto 2 de la Parte B de Fase 4 -- recién acá entra
// Bubbletea, y solo después del micro-checkpoint del cliente tonto
// (cmd/arg0s/daemon.go). La TUI es SOLO presentación (P1): arma el
// Model con un *ipc.Client y listo, ninguna decisión vive acá.
func newTUICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "TUI en vivo -- consume eventos de arg0sd, sin lógica de negocio (Fase 4 parte B)",
	}
	cmd.AddCommand(newTUISubmitCmd())
	cmd.AddCommand(newTUIAttachCmd())
	return cmd
}

func newTUISubmitCmd() *cobra.Command {
	var model string
	cmd := &cobra.Command{
		Use:   "submit <prompt>",
		Short: "Manda una task nueva a arg0sd y la muestra en vivo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := dialDaemon()
			if err != nil {
				return err
			}
			defer client.Close()

			taskID, err := client.Submit(context.Background(), args[0], model)
			if err != nil {
				return err
			}
			return runTUI(client, taskID, 0)
		},
		SilenceUsage: true,
	}
	cmd.Flags().StringVar(&model, "model", "", "modelo a usar")
	return cmd
}

// newTUIAttachCmd reengancha con una task viva o terminada en arg0sd --
// "cerrar la TUI a mitad de una task y reabrirla" (DoD Fase 4) es
// literalmente esto: la task sigue corriendo en arg0sd (otro proceso),
// attach vuelve a suscribirse desde el principio y reconstruye el
// estado completo.
func newTUIAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <task_id>",
		Short: "Reengancha con una task existente en arg0sd (viva o terminada) y reconstruye su estado completo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := dialDaemon()
			if err != nil {
				return err
			}
			defer client.Close()
			return runTUI(client, args[0], 0)
		},
		SilenceUsage: true,
	}
}

func runTUI(client *ipc.Client, taskID string, from int) error {
	m := tui.New(client, taskID)
	p := tea.NewProgram(m)

	if err := client.Subscribe(context.Background(), taskID, from, func(e ipc.Event) {
		p.Send(e)
	}); err != nil {
		return err
	}

	_, err := p.Run()
	return err
}
