package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/storage"
)

func newRunShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Muestra tokens, costo, latencia y modelo de un run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := config.Home()
			if err != nil {
				return err
			}
			db, err := storage.Open(filepath.Join(home, "arg0s.db"))
			if err != nil {
				return err
			}
			defer db.Close()

			run, modelRuns, err := db.GetRun(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Run %s\n", run.ID)
			fmt.Fprintf(out, "  prompt     %s\n", run.Prompt)
			fmt.Fprintf(out, "  strategy   %s\n", run.Strategy)
			fmt.Fprintf(out, "  status     %s\n", run.Status)
			fmt.Fprintf(out, "  tokens     %d in / %d out\n", run.InputTokens, run.OutputTokens)
			fmt.Fprintf(out, "  cost       $%.6f\n", run.CostUSD)
			fmt.Fprintf(out, "  duración   %s\n", run.EndedAt.Sub(run.StartedAt))
			if run.Error != "" {
				fmt.Fprintf(out, "  error      %s\n", run.Error)
			}

			fmt.Fprintln(out, "\n  model_runs")
			for _, mr := range modelRuns {
				fellBack := ""
				if mr.FellBackFrom != "" {
					fellBack = fmt.Sprintf(" (fallback desde %s)", mr.FellBackFrom)
				}
				fmt.Fprintf(out, "    %-20s %-10s %4dms  %d intento(s)  %d in / %d out  $%.6f  %s%s\n",
					mr.ModelID, mr.Role, mr.LatencyMS, mr.Attempts, mr.InputTokens, mr.OutputTokens, mr.CostUSD, mr.Status, fellBack)
				if mr.Error != "" {
					fmt.Fprintf(out, "      error: %s\n", mr.Error)
				}
			}
			return nil
		},
		SilenceUsage: true,
	}
}
