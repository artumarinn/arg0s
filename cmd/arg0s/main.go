// Command arg0s es el cliente (CLI + TUI) de Arg0s. En Fase 0 es un
// binario único sin daemon — arg0sd llega en Fase 4.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "arg0s",
		Short: "Arg0s — orquestador local de tareas de IA",
	}
	root.AddCommand(newVersionCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newProvidersCmd())
	root.AddCommand(newModelsCmd())
	root.AddCommand(newCostCmd())
	root.AddCommand(newRouteCmd())
	root.AddCommand(newMemoryCmd())
	root.AddCommand(newGraphCmd())
	root.AddCommand(newContextCmd())
	root.AddCommand(newBenchCmd())
	return root
}
