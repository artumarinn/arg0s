package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/router"
	"github.com/artumarinn/arg0s/internal/storage"
)

func newRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Inspecciona decisiones del router",
	}
	cmd.AddCommand(newRouteExplainCmd())
	return cmd
}

func newRouteExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain <prompt>",
		Short: "Perfila un prompt y muestra la decisión del router sin ejecutar nada",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			models, err := config.LoadModels()
			if err != nil {
				return err
			}
			home, err := config.Home()
			if err != nil {
				return err
			}
			db, err := storage.Open(filepath.Join(home, "arg0s.db"))
			if err != nil {
				return err
			}
			defer db.Close()

			exec := buildExecutor(cfg, models, nil)
			r := buildRouter(cfg, models, db, exec)

			decision, err := r.Decide(cmd.Context(), "explain", args[0])
			if err != nil {
				return err
			}

			printRoutingDecision(cmd.OutOrStdout(), decision)
			return nil
		},
		SilenceUsage: true,
	}
}

func printRoutingDecision(out io.Writer, d router.RoutingDecision) {
	fmt.Fprintf(out, "perfil        type=%s complexity=%s privacy=%s\n", d.Profile.Type, d.Profile.Complexity, d.Profile.Privacy)
	fmt.Fprintf(out, "mode          %s (confidence=%.2f)\n", d.Mode, d.ClassifierConfidence)
	fmt.Fprintf(out, "candidatos    %v\n", d.Candidates)
	fmt.Fprintf(out, "seleccionado  modelo=%s strategy=%s\n", d.SelectedModel, d.SelectedStrategy)

	fmt.Fprintln(out, "razones")
	for _, reason := range d.Reasons {
		fmt.Fprintf(out, "  - %s\n", reason)
	}

	if len(d.Rejected) > 0 {
		fmt.Fprintln(out, "rechazados")
		for _, rej := range d.Rejected {
			fmt.Fprintf(out, "  - %s: %s\n", rej.Model, rej.Reason)
		}
	}
}
