package strategies

import (
	"context"
	"time"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/fusion"
)

// uncertaintyThreshold: TaskProfile.Confidence por debajo de esto
// cuenta como "incertidumbre alta" para el gate de sección 10.1.
// Confidence == 0 (Profile nunca perfilado, ej. `arg0s run --model x`
// manual sin pasar por el router) NO cuenta como incertidumbre -- sería
// escalar fusion siempre que no haya router de por medio, que es
// exactamente el "1 request = 8 llamadas" que sección 10.1 quiere
// evitar.
const uncertaintyThreshold = 0.5

// Fusion es la estrategia de sección 10 -- componible con el Router
// (Router.Decide puede elegir SelectedStrategy="fusion"), pero
// funciona igual si el usuario la pide a mano (`arg0s run --strategy
// fusion`).
type Fusion struct {
	engine   *fusion.Engine
	modelID  string
	fallback string
}

func NewFusion(engine *fusion.Engine, modelID, fallback string) *Fusion {
	return &Fusion{engine: engine, modelID: modelID, fallback: fallback}
}

func (f *Fusion) Name() core.StrategyName { return "fusion" }

func (f *Fusion) Run(ctx context.Context, task *core.Task) (*core.Result, error) {
	started := time.Now()

	uncertain := task.Profile.Confidence > 0 && task.Profile.Confidence < uncertaintyThreshold
	result, err := f.engine.Run(ctx, task, f.modelID, f.fallback, uncertain)

	metadata := map[string]any{"fusion_decision": string(result.Decision), "fusion_escalated": len(result.ModelRuns) > 1, "fusion_winner": result.Winner}
	if result.Divergence != nil {
		metadata["fusion_divergence"] = result.Divergence.Score
		metadata["fusion_divergence_method"] = result.Divergence.Method
	}

	return &core.Result{
		TaskID: task.ID, Content: result.Content, Strategy: f.Name(),
		Usage: sumUsage(result.ModelRuns), ModelRuns: result.ModelRuns,
		StartedAt: started, EndedAt: time.Now(), Err: err, Metadata: metadata,
	}, err
}

// sumUsage agrega el costo/tokens de TODAS las llamadas de la corrida
// (primario + secundarios + synthesizer, los que hayan corrido) -- el
// costo real de una fusion que escaló es la suma, no solo el primario.
func sumUsage(runs []core.ModelRun) core.Usage {
	var total core.Usage
	for _, r := range runs {
		total.InputTokens += r.Usage.InputTokens
		total.OutputTokens += r.Usage.OutputTokens
		total.CachedTokens += r.Usage.CachedTokens
		total.CostUSD += r.Usage.CostUSD
	}
	return total
}
