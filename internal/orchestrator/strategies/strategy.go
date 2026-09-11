// Package strategies implementa las estrategias de ejecución (P3: son
// intercambiables, todas cumplen esta interfaz).
package strategies

import (
	"context"

	"github.com/artumarinn/arg0s/internal/core"
)

type StrategyName = core.StrategyName

// Strategy es un modo de ejecución de una Task (direct, router, fusion,
// judgment...). Ninguna estrategia habla con providers directamente —
// todas pasan por el Executor (P4).
type Strategy interface {
	Name() core.StrategyName
	Run(ctx context.Context, task *core.Task) (*core.Result, error)
}
