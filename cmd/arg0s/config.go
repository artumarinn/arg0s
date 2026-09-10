package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/artumarinn/arg0s/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Ver o modificar la configuración de arg0s",
	}
	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigSetCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Muestra la configuración efectiva (defaults + archivos + env)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			fmt.Fprint(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}

// ponytail: solo soporta las dos claves que ya tienen consumidor real
// (ver config.applyEnvOverrides). Un setter genérico por dotted-path
// sobre YAML arbitrario se agrega cuando haya más de dos claves que lo
// necesiten.
var configSettableKeys = map[string]func(*config.Config, string){
	"general.log_level":        func(c *config.Config, v string) { c.General.LogLevel = v },
	"general.default_strategy": func(c *config.Config, v string) { c.General.DefaultStrategy = v },
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Setea una clave en ~/.arg0s/config.yaml (" + configSettableKeysHelp() + ")",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			setter, ok := configSettableKeys[key]
			if !ok {
				return fmt.Errorf("clave desconocida %q (soportadas: %s)", key, configSettableKeysHelp())
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			setter(cfg, value)

			home, err := config.Home()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(home, 0o700); err != nil {
				return fmt.Errorf("crear %s: %w", home, err)
			}

			out, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}
			path := filepath.Join(home, "config.yaml")
			if err := os.WriteFile(path, out, 0o600); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s = %s escrito en %s\n", key, value, path)
			return nil
		},
	}
}

func configSettableKeysHelp() string {
	keys := make([]string, 0, len(configSettableKeys))
	for k := range configSettableKeys {
		keys = append(keys, k)
	}
	return fmt.Sprintf("%v", keys)
}
