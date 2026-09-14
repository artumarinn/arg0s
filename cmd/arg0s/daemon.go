package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/ipc"
)

// newDaemonCmd es el "cliente tonto" del micro-checkpoint de Fase 4
// parte B: imprime los eventos crudos que llegan por el socket, sin
// interpretarlos -- prueba que el streaming cross-process funciona
// ANTES de meter Bubbletea encima. No es la sintaxis literal
// "arg0s --daemon submit" de la nota de la Parte B; se implementó como
// subcomando (`arg0s daemon submit`) por prolijidad de cobra, mismo
// alcance.
func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Cliente IPC hacia arg0sd -- crudo, sin interpretar eventos (ver TUI para eso)",
	}
	cmd.AddCommand(newDaemonPingCmd())
	cmd.AddCommand(newDaemonSubmitCmd())
	cmd.AddCommand(newDaemonCancelCmd())
	return cmd
}

func dialDaemon() (*ipc.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	socketPath, err := cfg.SocketPath()
	if err != nil {
		return nil, err
	}
	client, err := ipc.Dial(socketPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a arg0sd en %s -- ¿está corriendo? (%w)", socketPath, err)
	}
	return client, nil
}

func newDaemonPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Chequea que arg0sd esté vivo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := dialDaemon()
			if err != nil {
				return err
			}
			defer client.Close()
			if err := client.Ping(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "pong")
			return nil
		},
		SilenceUsage: true,
	}
}

func newDaemonSubmitCmd() *cobra.Command {
	var model string
	cmd := &cobra.Command{
		Use:   "submit <prompt>",
		Short: "Manda una task a arg0sd y muestra los eventos crudos (chunk/done/failed/cancelled) tal como llegan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := dialDaemon()
			if err != nil {
				return err
			}
			defer client.Close()

			ctx := cmd.Context()
			taskID, err := client.Submit(ctx, args[0], model)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "task_id: %s\n", taskID)

			out := cmd.OutOrStdout()
			terminal := make(chan struct{})
			err = client.Subscribe(ctx, taskID, 0, func(e ipc.Event) {
				switch e.Type {
				case ipc.EventChunk:
					fmt.Fprintf(out, "[chunk] %s", e.Delta)
				case ipc.EventDone:
					fmt.Fprintln(out, "\n[done]")
					close(terminal)
				case ipc.EventFailed:
					fmt.Fprintf(out, "\n[failed] %s\n", e.Error)
					close(terminal)
				case ipc.EventCancelled:
					fmt.Fprintln(out, "\n[cancelled]")
					close(terminal)
				}
			})
			if err != nil {
				return err
			}

			<-terminal // el evento terminal SIEMPRE llega explícito -- nunca se infiere del cierre del socket
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().StringVar(&model, "model", "", "modelo a usar")
	return cmd
}

func newDaemonCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <task_id>",
		Short: "Cancela una task viva en arg0sd",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := dialDaemon()
			if err != nil {
				return err
			}
			defer client.Close()
			if err := client.Cancel(context.Background(), args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "cancelado")
			return nil
		},
		SilenceUsage: true,
	}
}
