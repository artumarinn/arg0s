package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version se fija en build time con -ldflags "-X main.version=...".
// "dev" es el default para builds locales sin ese flag.
var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Muestra la versión de arg0s",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "arg0s", version)
			return nil
		},
	}
}
