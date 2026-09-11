package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/storage"
)

func newCostCmd() *cobra.Command {
	var today bool
	var byModel bool
	var last string

	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Costo y tokens gastados",
		RunE: func(cmd *cobra.Command, args []string) error {
			since, err := costSince(today, last)
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

			out := cmd.OutOrStdout()
			if byModel {
				costs, err := db.CostByModel(cmd.Context(), since)
				if err != nil {
					return err
				}
				for _, c := range costs {
					fmt.Fprintf(out, "%-20s $%.6f  (%d runs)\n", c.ModelID, c.CostUSD, c.Runs)
				}
				return nil
			}

			summary, err := db.CostSummary(cmd.Context(), since)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "runs:          %d\n", summary.Runs)
			fmt.Fprintf(out, "input tokens:  %d\n", summary.InputTokens)
			fmt.Fprintf(out, "output tokens: %d\n", summary.OutputTokens)
			fmt.Fprintf(out, "cost:          $%.6f\n", summary.CostUSD)
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().BoolVar(&today, "today", false, "desde el inicio del día de hoy")
	cmd.Flags().BoolVar(&byModel, "by-model", false, "agrupar por modelo")
	cmd.Flags().StringVar(&last, "last", "", "ventana relativa, ej 7d, 24h")
	return cmd
}

// costSince resuelve --today / --last <ventana> a un instante "desde".
// Sin flags, default a hoy (mismo default que el ejemplo de la sección
// 21: `arg0s cost --today`).
func costSince(today bool, last string) (time.Time, error) {
	now := time.Now()
	if last != "" {
		d, err := parseRelativeDuration(last)
		if err != nil {
			return time.Time{}, err
		}
		return now.Add(-d), nil
	}
	if today {
		y, m, d := now.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location()), nil
	}
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()), nil
}

// parseRelativeDuration soporta "7d" además de lo que ya entiende
// time.ParseDuration (que no tiene unidad de día).
func parseRelativeDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("ventana inválida %q: %w", s, err)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
