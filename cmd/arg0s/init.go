package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/storage"
)

// initTargetDirs son los directorios que `arg0s init` crea bajo home
// (incluyendo el propio home).
func initTargetDirs(home string) []string {
	return []string{
		home,
		filepath.Join(home, "skills", "native"),
		filepath.Join(home, "skills", "external"),
		filepath.Join(home, "projects"),
		filepath.Join(home, "cache"),
		filepath.Join(home, "logs"),
	}
}

func newInitCmd() *cobra.Command {
	var force bool
	var homeFlag string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Crea ~/.arg0s/ (config.yaml, models.yaml, .env, arg0s.db, directorios)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home := homeFlag
			if home == "" {
				h, err := config.Home()
				if err != nil {
					return err
				}
				home = h
			}
			return runInit(cmd, home, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "sobrescribe ~/.arg0s/ existente")
	cmd.Flags().StringVar(&homeFlag, "home", "", "usar este directorio en vez de ~/.arg0s/ (o $ARG0S_HOME)")
	return cmd
}

func runInit(cmd *cobra.Command, home string, force bool) error {
	if !force {
		if existing, err := existingEntries(home); err != nil {
			return err
		} else if len(existing) > 0 {
			return fmt.Errorf("%s ya existe con: %v (usá --force para sobrescribir)", home, existing)
		}
	}

	for _, dir := range initTargetDirs(home) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	configPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(configPath, config.DefaultConfigYAML(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}

	modelsPath := filepath.Join(home, "models.yaml")
	if err := os.WriteFile(modelsPath, config.DefaultModelsYAML(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", modelsPath, err)
	}

	envPath := filepath.Join(home, ".env")
	if err := os.WriteFile(envPath, config.DefaultEnvTemplate(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", envPath, err)
	}

	dbPath := filepath.Join(home, "arg0s.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		return fmt.Errorf("init db %s: %w", dbPath, err)
	}
	db.Close()

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "arg0s init: %s listo\n", home)
	fmt.Fprintln(out, "  config.yaml, models.yaml, .env, arg0s.db")
	fmt.Fprintln(out, "  skills/, projects/, cache/, logs/")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Para llegar a verde en `arg0s doctor` todavía falta:")
	fmt.Fprintf(out, "  - cargar GEMINI_API_KEY en %s\n", envPath)
	fmt.Fprintln(out, "  - tener ollama corriendo (ollama serve) si vas a usar modelos locales")
	fmt.Fprintln(out, "  - los IDs de models.yaml son placeholders: correr `arg0s models sync` cuando exista (Fase 1)")

	return nil
}

// existingEntries devuelve los nombres del contenido actual de home, si
// el directorio ya existe. Un home ausente no es conflicto — devuelve
// nil, nil. Sección 21: "si ~/.arg0s/ existe, error claro diciendo qué
// archivos ya están".
func existingEntries(home string) ([]string, error) {
	entries, err := os.ReadDir(home)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", home, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	return names, nil
}
