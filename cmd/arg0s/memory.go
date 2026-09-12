package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/memory"
	memsqlite "github.com/artumarinn/arg0s/internal/memory/sqlite"
	"github.com/artumarinn/arg0s/internal/storage"
)

// buildMemoryStore abre arg0s.db (ya migrado por storage.Open, que
// crea la tabla `memories`) y arma el Store sobre ese mismo *sql.DB --
// mismo patrón que telemetry.NewBus(db.DB).
func buildMemoryStore(cfg *config.Config, db *storage.DB) *memsqlite.Store {
	return memsqlite.New(db.DB, parseRevalidateAfter(cfg.Memory.Decay.RevalidateAfter))
}

// parseRevalidateAfter soporta el sufijo "d" (días) que usa
// memory.decay.revalidate_after en config.yaml -- time.ParseDuration
// no lo entiende nativamente. Vacío o inválido = decaimiento
// desactivado (Store trata revalidateAfter<=0 como "sin decaimiento").
func parseRevalidateAfter(s string) time.Duration {
	if strings.HasSuffix(s, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(s, "d") + "h")
		if err != nil {
			return 0
		}
		return days * 24
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// projectFor determina el `project` de una memoria -- el cwd, igual
// que sessions.project (internal/storage/runs.go). memory.scope
// (project|global|session) queda para cuando context compiler lo
// necesite (Fase 3 parte B); acá todo es scope=project.
func projectFor() string {
	cwd, _ := filepath.Abs(".")
	return cwd
}

// openMemoryDB es para `add`/`forget` -- son mutadores explícitos
// (regla dura #11), así que crear arg0s.db si no existe (storage.Open
// lo hace) es el comportamiento correcto, igual que `arg0s run`.
func openMemoryDB(cmd *cobra.Command) (*storage.DB, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	home, err := config.Home()
	if err != nil {
		return nil, nil, err
	}
	db, err := storage.Open(filepath.Join(home, "arg0s.db"))
	if err != nil {
		return nil, nil, err
	}
	return db, cfg, nil
}

// openReadOnlyMemoryDB es para `list`/`search` -- son de consulta
// (regla dura #11): nunca crean arg0s.db como side effect. Si no
// existe, lo reportan y dicen qué comando lo crea, en vez de abrirlo y
// dejar un arg0s.db huérfano (mismo bug que ya se corrigió en
// `doctor`, ver Engram arg0s/preference/read-only-diagnostics).
func openReadOnlyMemoryDB(cmd *cobra.Command) (*storage.DB, *config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	home, err := config.Home()
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(home, "arg0s.db")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("arg0s.db no existe -- correr `arg0s init` (o `arg0s memory add` para crear la primera memoria)")
		}
		return nil, nil, err
	}
	db, err := storage.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return db, cfg, nil
}

func newMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Memoria persistente de Arg0s (sección 12)",
	}
	cmd.AddCommand(newMemoryAddCmd())
	cmd.AddCommand(newMemoryListCmd())
	cmd.AddCommand(newMemorySearchCmd())
	cmd.AddCommand(newMemoryForgetCmd())
	cmd.AddCommand(newMemoryRevalidateCmd())
	return cmd
}

// newMemoryRevalidateCmd es la ÚNICA vía para persistir `stale` --
// Query() lo calcula al vuelo pero nunca lo escribe (una consulta no
// muta estado, regla dura #11). Es un mutador explícito, así que usa
// openMemoryDB (puede crear arg0s.db), igual que add/forget.
func newMemoryRevalidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revalidate",
		Short: "Persiste stale=1 en las memorias que ya vencieron memory.decay.revalidate_after",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, cfg, err := openMemoryDB(cmd)
			if err != nil {
				return err
			}
			defer db.Close()

			marked, err := buildMemoryStore(cfg, db).Revalidate(cmd.Context(), projectFor())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d memoria(s) marcada(s) stale\n", marked)
			return nil
		},
		SilenceUsage: true,
	}
}

func newMemoryAddCmd() *cobra.Command {
	var category, source string
	var refs []string
	var confidence float64

	cmd := &cobra.Command{
		Use:   "add <título> <contenido>",
		Short: "Agrega una memoria",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, _, err := openMemoryDB(cmd)
			if err != nil {
				return err
			}
			defer db.Close()

			store := memsqlite.New(db.DB, 0)
			m := memory.Memory{
				Project: projectFor(), Category: memory.Category(strings.ToUpper(category)),
				Title: args[0], Content: args[1], Confidence: confidence, Source: source, Refs: refs,
			}
			if err := store.Store(cmd.Context(), m); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "memoria guardada")
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().StringVar(&category, "category", string(memory.CategoryFact), "DECISION|FACT|BUG|PREFERENCE|ARCHITECTURE|TODO|LESSON")
	cmd.Flags().StringVar(&source, "source", "user", "origen de la memoria")
	cmd.Flags().StringSliceVar(&refs, "ref", nil, "archivos/símbolos/commits relacionados (repetible)")
	cmd.Flags().Float64Var(&confidence, "confidence", 1.0, "confianza (0..1)")
	return cmd
}

func newMemoryListCmd() *cobra.Command {
	var limit int
	var includeStale bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista memorias del proyecto actual",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, cfg, err := openReadOnlyMemoryDB(cmd)
			if err != nil {
				return err
			}
			defer db.Close()

			if limit == 0 {
				limit = cfg.Memory.MaxFragmentsPerQuery
			}
			store := buildMemoryStore(cfg, db)
			results, err := store.Query(cmd.Context(), memory.MemoryQuery{
				Project: projectFor(), Limit: limit, IncludeStale: includeStale,
			})
			if err != nil {
				return err
			}
			printMemories(cmd, results)
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "máximo de resultados (0 = memory.max_fragments_per_query)")
	cmd.Flags().BoolVar(&includeStale, "include-stale", false, "incluir memorias marcadas stale")
	return cmd
}

func newMemorySearchCmd() *cobra.Command {
	var limit int
	var minRelevance float64
	var includeStale bool

	cmd := &cobra.Command{
		Use:   "search <texto>",
		Short: "Busca memorias por relevancia lexical (sin embeddings, ver internal/memory/sqlite)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, cfg, err := openReadOnlyMemoryDB(cmd)
			if err != nil {
				return err
			}
			defer db.Close()

			if limit == 0 {
				limit = cfg.Memory.MaxFragmentsPerQuery
			}
			effectiveMinRelevance := minRelevance
			if !cmd.Flags().Changed("min-relevance") {
				effectiveMinRelevance = cfg.Memory.MinRelevance
			}

			store := buildMemoryStore(cfg, db)
			results, err := store.Query(cmd.Context(), memory.MemoryQuery{
				Project: projectFor(), Text: args[0], Limit: limit,
				MinRelevance: effectiveMinRelevance, IncludeStale: includeStale,
			})
			if err != nil {
				return err
			}
			printMemories(cmd, results)
			return nil
		},
		SilenceUsage: true,
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "máximo de resultados (0 = memory.max_fragments_per_query)")
	cmd.Flags().Float64Var(&minRelevance, "min-relevance", 0, "piso de relevancia (default: memory.min_relevance)")
	cmd.Flags().BoolVar(&includeStale, "include-stale", false, "incluir memorias marcadas stale")
	return cmd
}

func newMemoryForgetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "forget <id>",
		Short: "Borra una memoria (irreversible -- distinto de stale, que nunca se borra solo)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, _, err := openMemoryDB(cmd)
			if err != nil {
				return err
			}
			defer db.Close()

			store := memsqlite.New(db.DB, 0)
			if err := store.Forget(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "memoria borrada")
			return nil
		},
		SilenceUsage: true,
	}
}

func printMemories(cmd *cobra.Command, results []memory.Memory) {
	out := cmd.OutOrStdout()
	if len(results) == 0 {
		fmt.Fprintln(out, "sin resultados")
		return
	}
	for _, m := range results {
		stale := ""
		if m.Stale {
			stale = " [stale]"
		}
		fmt.Fprintf(out, "%s  [%s]%s  %s\n", m.ID, m.Category, stale, m.Title)
		fmt.Fprintf(out, "    %s\n", m.Content)
		if len(m.Refs) > 0 {
			fmt.Fprintf(out, "    refs: %s\n", strings.Join(m.Refs, ", "))
		}
	}
}
