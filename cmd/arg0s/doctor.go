package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/storage"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Valida config.yaml, models.yaml y storage; reporta ✓/✗/⊘ por check",
		RunE: func(cmd *cobra.Command, args []string) error {
			checks, hasFail, err := runDoctorChecks()
			if err != nil {
				return err
			}
			printDoctorReport(cmd, checks)
			if hasFail {
				return errDoctorFailed
			}
			return nil
		},
		SilenceUsage: true,
	}
}

var errDoctorFailed = fmt.Errorf("doctor encontró checks fallidos")

// runDoctorChecks agrega los checks de la sección 6.7. Desde Fase 1,
// los checks 6 y 7 (conectividad real, modelos de Ollama cargados) los
// resuelve providers.Registry.Health() de verdad -- ya no son skip.
// Los checks 9 y 10 (binarios de runtime, servidores LSP) siguen sin
// implementación (Fase 6 y Fase 3 respectivamente).
func runDoctorChecks() (checks []core.Check, hasFail bool, err error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, false, fmt.Errorf("load config: %w", err)
	}
	models, err := config.LoadModels()
	if err != nil {
		return nil, false, fmt.Errorf("load models: %w", err)
	}

	checks = append(checks, config.Validate(cfg, models)...)
	checks = append(checks, buildRegistry(cfg).Health(context.Background(), 10*time.Second)...)
	checks = append(checks, notImplementedSkipChecks(cfg)...)

	home, err := config.Home()
	if err != nil {
		return nil, false, err
	}
	checks = append(checks, storageCheck(filepath.Join(home, "arg0s.db"))...)

	for _, c := range checks {
		if c.Status == core.CheckFail {
			hasFail = true
		}
	}
	return checks, hasFail, nil
}

// storageCheck es de solo lectura: doctor nunca debe crear arg0s.db ni
// ~/.arg0s/ como side effect (eso rompería la detección de conflicto de
// `arg0s init` en la corrida siguiente). Si el archivo no existe, lo
// reporta sin tocarlo; si existe, sí lo abre para chequear migraciones.
func storageCheck(path string) []core.Check {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return []core.Check{{Group: "Storage", Name: "arg0s.db", Status: core.CheckFail, Detail: "no existe (correr arg0s init)"}}
		}
		return []core.Check{{Group: "Storage", Name: "arg0s.db", Status: core.CheckFail, Detail: err.Error()}}
	}

	db, err := storage.Open(path)
	if err != nil {
		return []core.Check{{Group: "Storage", Name: "arg0s.db", Status: core.CheckFail, Detail: err.Error()}}
	}
	defer db.Close()
	return db.Health("arg0s.db")
}

// notImplementedSkipChecks cubre los checks 9 y 10 de la sección 6.7 —
// todavía sin implementación real (Fase 6 y Fase 3 respectivamente).
func notImplementedSkipChecks(cfg *config.Config) []core.Check {
	var checks []core.Check
	for _, name := range sortedRuntimeNames(cfg) {
		r := cfg.Runtimes[name]
		if !r.Enabled {
			continue
		}
		checks = append(checks, core.Check{Group: "Runtimes", Name: name + " binary", Status: core.CheckSkip, Detail: "not implemented yet (Fase 6)"})
	}
	checks = append(checks, core.Check{Group: "CodeGraph", Name: "LSP servers", Status: core.CheckSkip, Detail: "not implemented yet (Fase 3)"})
	return checks
}

func sortedProviderNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Providers))
	for n := range cfg.Providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func sortedRuntimeNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Runtimes))
	for n := range cfg.Runtimes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func printDoctorReport(cmd *cobra.Command, checks []core.Check) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "ARG0S DOCTOR")

	groups := []string{"Config", "Providers", "Roles", "Runtimes", "CodeGraph", "Storage"}
	byGroup := map[string][]core.Check{}
	for _, c := range checks {
		byGroup[c.Group] = append(byGroup[c.Group], c)
	}

	errors, warnings := 0, 0
	for _, g := range groups {
		items := byGroup[g]
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(out, "\n%s\n", g)
		for _, c := range items {
			symbol := "✓"
			switch c.Status {
			case core.CheckFail:
				symbol = "✗"
				errors++
			case core.CheckSkip:
				symbol = "⊘"
			}
			fmt.Fprintf(out, "  %s %-28s %s\n", symbol, c.Name, c.Detail)
		}
	}

	fmt.Fprintf(out, "\n%d error(s), %d warning(s)\n", errors, warnings)
}
