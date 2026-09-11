package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
)

func newModelsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "models", Short: "Ver y sincronizar el catálogo de modelos"}
	cmd.AddCommand(newModelsListCmd())
	cmd.AddCommand(newModelsTestCmd())
	cmd.AddCommand(newModelsSyncCmd())
	cmd.AddCommand(newModelsCostCmd())
	return cmd
}

func newModelsListCmd() *cobra.Command {
	var providerFilter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista el catálogo (models.yaml)",
		RunE: func(cmd *cobra.Command, args []string) error {
			models, err := config.LoadModels()
			if err != nil {
				return err
			}
			ids := make([]string, 0, len(models.Models))
			for id := range models.Models {
				ids = append(ids, id)
			}
			sort.Strings(ids)

			out := cmd.OutOrStdout()
			for _, id := range ids {
				m := models.Models[id]
				if providerFilter != "" && m.Provider != providerFilter {
					continue
				}
				fmt.Fprintf(out, "%-20s %-12s %-10s tier=%-8s in=$%.2f/1M out=$%.2f/1M\n",
					id, m.Provider, m.ProviderModelID, m.Tier, m.Cost.InputPer1MUSD, m.Cost.OutputPer1MUSD)
			}
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().StringVar(&providerFilter, "provider", "", "filtrar por provider")
	return cmd
}

func newModelsTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <id>",
		Short: "Prueba un modelo con un prompt mínimo",
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
			exec := buildExecutor(cfg, models, nil)

			start := time.Now()
			resp, _, err := exec.Execute(context.Background(), core.Request{ModelID: args[0], Prompt: "ping", Role: core.RoleGenerator})
			latency := time.Since(start)

			out := cmd.OutOrStdout()
			if err != nil {
				fmt.Fprintf(out, "✗ %s: %s\n", args[0], core.RedactSecrets(err.Error()))
				return err
			}
			fmt.Fprintf(out, "✓ %s reachable %dms: %q\n", args[0], latency.Milliseconds(), truncate(resp.Content, 80))
			return nil
		},
		SilenceUsage: true,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func newModelsSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Consulta cada provider habilitado y agrega los modelos que falten a models.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			models, err := config.LoadModels()
			if err != nil {
				return err
			}
			if models.Models == nil {
				models.Models = map[string]config.ModelConfig{}
			}

			reg := buildRegistry(cfg)
			out := cmd.OutOrStdout()
			added := 0
			for _, name := range reg.Names() {
				p, _ := reg.Get(name)
				remoteModels, err := p.Models(context.Background())
				if err != nil {
					fmt.Fprintf(out, "⊘ %s: %s\n", name, core.RedactSecrets(err.Error()))
					continue
				}
				for _, m := range remoteModels {
					if _, exists := models.Models[m.ID]; exists {
						continue
					}
					models.Models[m.ID] = config.ModelConfig{
						Provider: m.Provider, ProviderModelID: m.ProviderModelID, Local: m.Local,
						Notes: "agregado por `arg0s models sync` -- completar tier/cost/context_window a mano",
					}
					added++
				}
			}

			home, err := config.Home()
			if err != nil {
				return err
			}
			path := filepath.Join(home, "models.yaml")
			data, err := yaml.Marshal(models)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				return err
			}
			fmt.Fprintf(out, "%d modelo(s) nuevo(s) agregado(s) a %s\n", added, path)
			return nil
		},
		SilenceUsage: true,
	}
}

func newModelsCostCmd() *cobra.Command {
	var tokens int
	cmd := &cobra.Command{
		Use:   "cost <id>",
		Short: "Costo estimado de N tokens con ese modelo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			models, err := config.LoadModels()
			if err != nil {
				return err
			}
			real := models.ResolveModel(args[0])
			m, ok := models.Models[real]
			if !ok {
				return fmt.Errorf("modelo %q no está en models.yaml", args[0])
			}

			inCost := float64(tokens) / 1_000_000 * m.Cost.InputPer1MUSD
			outCost := float64(tokens) / 1_000_000 * m.Cost.OutputPer1MUSD
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s -- %d tokens\n", real, tokens)
			fmt.Fprintf(out, "  como input:  $%.6f\n", inCost)
			fmt.Fprintf(out, "  como output: $%.6f\n", outCost)
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().IntVar(&tokens, "tokens", 1000, "cantidad de tokens")
	return cmd
}
