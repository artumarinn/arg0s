package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/contextc"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/orchestrator/strategies"
)

func newBenchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Benchmarks propios de Arg0s, siempre contra un baseline explícito (sección 17)",
	}
	cmd.AddCommand(newBenchContextCmd())
	cmd.AddCommand(newBenchFusionCmd())
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

// defaultFusionBenchPrompt fuerza complexity=high a mano (no depende
// del router) para que el gate de sección 10.1 escale siempre -- el
// benchmark mide el ENSEMBLE, no si el router decidió activarlo.
const defaultFusionBenchPrompt = "diseñá el manejo de errores para un pipeline de pagos con reintentos, idempotencia y compensación"

func newBenchFusionCmd() *cobra.Command {
	var prompt string

	cmd := &cobra.Command{
		Use:   "fusion",
		Short: "Costo del ensemble + substitution_rate medido vs. primario solo -- ensemble_gain real queda pendiente de judge (sección 17.2)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			models, err := config.LoadModels()
			if err != nil {
				return err
			}
			if len(cfg.Fusion.Models) == 0 {
				return fmt.Errorf("fusion.models está vacío en config.yaml -- no hay con qué comparar")
			}
			primaryModel := cfg.Fusion.Models[0]

			exec := buildExecutor(cfg, models, nil)
			task := &core.Task{Prompt: prompt, Profile: core.TaskProfile{Complexity: core.LevelHigh}}

			baseline, err := strategies.NewDirect(exec, primaryModel, "").Run(cmd.Context(), task)
			if err != nil {
				return fmt.Errorf("baseline (primario solo): %w", err)
			}

			fusionResult, err := strategies.NewFusion(buildFusionEngine(cfg, exec), primaryModel, "").Run(cmd.Context(), task)
			if err != nil {
				return fmt.Errorf("fusion: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "prompt: %q\n\n", prompt)
			fmt.Fprintf(out, "baseline (solo %s):  %d llamadas, $%.6f, %d+%d tokens\n",
				primaryModel, len(baseline.ModelRuns), baseline.Usage.CostUSD, baseline.Usage.InputTokens, baseline.Usage.OutputTokens)
			fmt.Fprintf(out, "fusion (adaptativo):    %d llamadas, $%.6f, %d+%d tokens\n",
				len(fusionResult.ModelRuns), fusionResult.Usage.CostUSD, fusionResult.Usage.InputTokens, fusionResult.Usage.OutputTokens)
			fmt.Fprintf(out, "costo extra del ensemble: $%.6f (%d llamada(s) extra)\n\n",
				fusionResult.Usage.CostUSD-baseline.Usage.CostUSD, len(fusionResult.ModelRuns)-len(baseline.ModelRuns))

			printFusionDecision(out, fusionResult.Metadata)
			fmt.Fprintln(out)

			winner, _ := fusionResult.Metadata["fusion_winner"].(int)
			switch winner {
			case 0:
				fmt.Fprintln(out, "substitution_rate: no aplica (no hubo síntesis -- no escaló, o divergencia fuera de la banda de síntesis)")
			case 1:
				fmt.Fprintln(out, "substitution_rate: 0% -- el synthesizer eligió al primario igual (1 corrida)")
			default:
				fmt.Fprintln(out, "substitution_rate: 100% -- el synthesizer prefirió un secundario sobre el primario (1 corrida)")
			}
			fmt.Fprintln(out, "ensemble_gain:     pendiente -- requiere Judgment Day (Fase 5) o revisión humana, sección 17.2. No confundir con substitution_rate: sustitución mide ACTIVIDAD del synthesizer (a qué candidata le dio la razón), no CALIDAD verificada de esa elección.")
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().StringVar(&prompt, "prompt", defaultFusionBenchPrompt, "prompt de referencia (default: caso concreto que fuerza escalado)")
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
