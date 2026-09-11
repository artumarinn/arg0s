package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
)

func newProvidersCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "providers", Short: "Ver estado de los providers configurados"}
	cmd.AddCommand(newProvidersListCmd())
	cmd.AddCommand(newProvidersTestCmd())
	return cmd
}

func newProvidersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Lista providers, tipo y estado",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, name := range sortedProviderNames(cfg) {
				p := cfg.Providers[name]
				status := "disabled"
				if p.Enabled {
					status = "enabled"
				}
				fmt.Fprintf(out, "%-16s %-20s %s\n", name, p.Type, status)
			}
			return nil
		},
		SilenceUsage: true,
	}
}

func newProvidersTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test <name>",
		Short: "Ping real (Models()) contra un provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name := args[0]
			p, ok := cfg.Providers[name]
			if !ok {
				return fmt.Errorf("provider %q no está en config.yaml", name)
			}
			if !p.Enabled {
				return fmt.Errorf("provider %q está disabled", name)
			}

			reg := buildRegistry(cfg)
			provider, ok := reg.Get(name)
			if !ok {
				return fmt.Errorf("provider %q (type=%s) no tiene implementación registrada", name, p.Type)
			}

			timeout := parseDuration(p.Timeout, 30*time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			start := time.Now()
			models, err := provider.Models(ctx)
			latency := time.Since(start)

			out := cmd.OutOrStdout()
			if err != nil {
				fmt.Fprintf(out, "✗ %s: %s\n", name, core.RedactSecrets(err.Error()))
				return err
			}
			fmt.Fprintf(out, "✓ %s reachable %dms (%d modelos)\n", name, latency.Milliseconds(), len(models))
			return nil
		},
		SilenceUsage: true,
	}
}
