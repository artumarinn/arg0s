package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/contextc"
)

func newBenchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmarks propios de Arg0s, siempre contra un baseline explícito (sección 17)",
	}
	cmd.AddCommand(newBenchContextCmd())
	return cmd
}

// defaultBenchPrompt es el caso concreto de sección 17.2 pedido en el
// DoD de Fase 3 -- un prompt real sobre este mismo repo, no un
// ejemplo sintético.
const defaultBenchPrompt = "refactor el TaskProfile en internal/router"

func newBenchContextCmd() *cobra.Command {
	var compareBaseline bool
	var prompt string

	cmd := &cobra.Command{
		Use:   "context",
		Short: "Mide tokens reales del Context Compiler vs. mandar los archivos completos mencionados (baseline obligatorio, sección 17.2)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !compareBaseline {
				return fmt.Errorf("bench context requiere --compare-baseline -- sección 17.2: \"toda métrica se compara contra un baseline explícito, sin baseline el número no se reporta\"")
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			models, err := config.LoadModels()
			if err != nil {
				return err
			}
			db, err := openReadOnlyGraphDB()
			if err != nil {
				return err
			}
			defer db.Close()

			project, err := os.Getwd()
			if err != nil {
				return err
			}

			exec := buildExecutor(cfg, models, nil)
			compiler := buildContextCompiler(cfg, db, exec)

			cc, err := compiler.Compile(cmd.Context(), prompt, project)
			if err != nil {
				return err
			}

			compiledCodeTokens, baselineTokens, files := codeVsBaselineTokens(project, cc)

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "prompt: %q\n", prompt)
			fmt.Fprintf(out, "archivos tocados: %s\n\n", strings.Join(files, ", "))
			fmt.Fprintf(out, "baseline (archivos completos, sin compresión): %d tokens\n", baselineTokens)
			fmt.Fprintf(out, "arg0s (Context Compiler, rangos acotados):     %d tokens\n", compiledCodeTokens)

			if baselineTokens == 0 {
				fmt.Fprintln(out, "\nsin archivos de código en este caso -- nada que comparar (¿corriste `arg0s graph index`?)")
				return nil
			}
			saved := baselineTokens - compiledCodeTokens
			pct := float64(saved) / float64(baselineTokens) * 100
			fmt.Fprintf(out, "ahorro:                                         %d tokens (%.1f%%)\n", saved, pct)
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().BoolVar(&compareBaseline, "compare-baseline", false, "obligatorio -- compara contra mandar los archivos completos, sin esto no se reporta ningún número")
	cmd.Flags().StringVar(&prompt, "prompt", defaultBenchPrompt, "prompt de referencia (default: caso concreto de este repo)")
	return cmd
}

// codeVsBaselineTokens aísla la comparación al contenido de CÓDIGO
// (fuentes "codegraph"/"file") -- memory y skills son iguales en las
// dos alternativas (el Context Compiler no cambia cómo se buscan
// memorias), así que no aportan nada a esta métrica puntual y
// mezclarlos infla el número sin que sea la reducción real que aporta
// el grafo. compiledCodeTokens sale de sumar Fragment.Tokens (ya
// acotados/comprimidos); baselineTokens lee cada archivo distinto
// DE VERDAD desde disco y cuenta sus tokens completos -- ambos con
// contextc.EstimateTokens, la misma vara.
func codeVsBaselineTokens(project string, cc contextc.CompiledContext) (compiledCodeTokens, baselineTokens int, files []string) {
	seen := map[string]bool{}
	for _, f := range cc.Fragments {
		if f.Source != "codegraph" && f.Source != "file" {
			continue
		}
		compiledCodeTokens += f.Tokens

		path := f.Ref
		if f.Source == "codegraph" {
			if idx := strings.LastIndex(path, ":"); idx > 0 {
				path = path[:idx]
			}
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		files = append(files, path)

		data, err := os.ReadFile(filepath.Join(project, path))
		if err != nil {
			continue
		}
		baselineTokens += contextc.EstimateTokens(string(data))
	}
	return compiledCodeTokens, baselineTokens, files
}
