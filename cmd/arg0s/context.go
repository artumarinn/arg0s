package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/contextc"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Context Compiler: qué entra al prompt bajo presupuesto (sección 9)",
	}
	cmd.AddCommand(newContextPreviewCmd())
	return cmd
}

func newContextPreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview <prompt>",
		Short: "Compila el contexto para un prompt y muestra EXACTAMENTE qué entró, de qué fuente, cuántos tokens, y qué se descartó y por qué",
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
			db, err := openReadOnlyGraphDB() // solo-lectura: no crea arg0s.db (regla dura #11)
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

			cc, err := compiler.Compile(cmd.Context(), args[0], project)
			if err != nil {
				return err
			}
			printCompiledContext(cmd, cc)
			return nil
		},
		SilenceUsage: true,
	}
}

func printCompiledContext(cmd *cobra.Command, cc contextc.CompiledContext) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "intent: %s\n", cc.Intent)
	fmt.Fprintf(out, "budget: %d / %d tokens usados\n\n", cc.TotalTokens, cc.Budget)

	fmt.Fprintln(out, "incluido")
	for _, f := range cc.Fragments {
		fmt.Fprintf(out, "  [%-9s] %-40s %5d tokens (relevance=%.2f)\n", f.Source, f.Ref, f.Tokens, f.Relevance)
	}

	if len(cc.Manifest.Excluded) == 0 {
		fmt.Fprintln(out, "\ndescartado: nada")
		return
	}
	fmt.Fprintln(out, "\ndescartado")
	for _, ex := range cc.Manifest.Excluded {
		fmt.Fprintf(out, "  [%-9s] %-40s %5d tokens -- %s\n", ex.Source, ex.Ref, ex.Tokens, ex.Reason)
	}
}
