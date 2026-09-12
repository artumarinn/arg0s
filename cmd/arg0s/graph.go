package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/codegraph"
	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/storage"
)

func newGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Code graph: símbolos y relaciones del repo (sección 13)",
	}
	cmd.AddCommand(newGraphIndexCmd())
	cmd.AddCommand(newGraphQueryCmd())
	cmd.AddCommand(newGraphCallersCmd())
	return cmd
}

func newGraphIndexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "index",
		Short: "Indexa el repo actual con gopls (único LSP real de Fase 3)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
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

			gopls := cfg.CodeGraph.Servers["go"]
			if gopls.Command == "" {
				gopls.Command = "gopls"
				gopls.Args = []string{"serve"}
			}

			rootDir, err := os.Getwd()
			if err != nil {
				return err
			}

			stats, err := codegraph.Index(cmd.Context(), gopls.Command, gopls.Args, rootDir, rootDir, db.DB, cfg.CodeGraph.Ignore)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "archivos .go indexados: %d\n", stats.FilesIndexed)
			fmt.Fprintf(out, "symbols:                %d\n", stats.SymbolsIndexed)
			fmt.Fprintf(out, "edges (calls):          %d\n", stats.EdgesIndexed)
			for ext, n := range stats.SkippedLanguages {
				fmt.Fprintf(out, "⚠ %d archivo(s) %s sin indexar -- LSP no implementado para ese lenguaje en Fase 3 (confidence: low, sin fallback regex)\n", n, ext)
			}
			return nil
		},
		SilenceUsage: true,
	}
}

// openReadOnlyGraphDB es de consulta (regla dura #11): nunca crea
// arg0s.db. Mismo criterio que openReadOnlyMemoryDB.
func openReadOnlyGraphDB() (*storage.DB, error) {
	home, err := config.Home()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, "arg0s.db")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("arg0s.db no existe -- correr `arg0s graph index` primero")
		}
		return nil, err
	}
	return storage.Open(path)
}

func newGraphQueryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "query <symbol>",
		Short: "Busca un símbolo por nombre y muestra dónde está definido",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openReadOnlyGraphDB()
			if err != nil {
				return err
			}
			defer db.Close()

			project, err := os.Getwd()
			if err != nil {
				return err
			}
			results, err := codegraph.NewGraph(db.DB).FindSymbol(cmd.Context(), project, args[0])
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "sin resultados -- ¿corriste `arg0s graph index`?")
				return nil
			}
			for _, s := range results {
				printSymbol(cmd, s)
			}
			return nil
		},
		SilenceUsage: true,
	}
}

func newGraphCallersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "callers <symbol>",
		Short: "Lista quién llama a un símbolo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openReadOnlyGraphDB()
			if err != nil {
				return err
			}
			defer db.Close()

			project, err := os.Getwd()
			if err != nil {
				return err
			}
			graph := codegraph.NewGraph(db.DB)
			target, err := graph.Definition(cmd.Context(), project, args[0])
			if err != nil {
				return fmt.Errorf("%q no encontrado -- ¿corriste `arg0s graph index`?: %w", args[0], err)
			}
			callers, err := graph.Callers(cmd.Context(), project, target.ID, 1)
			if err != nil {
				return err
			}
			if len(callers) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "nadie llama a %s (o no se pudo resolver -- gopls references)\n", target.Name)
				return nil
			}
			for _, c := range callers {
				printSymbol(cmd, c)
			}
			return nil
		},
		SilenceUsage: true,
	}
}

func printSymbol(cmd *cobra.Command, s codegraph.Symbol) {
	fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s:%d-%d\n", s.Kind, s.Name, s.File, s.LineStart, s.LineEnd)
	if s.Signature != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", s.Signature)
	}
}
