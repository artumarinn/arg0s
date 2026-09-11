package strategies

import (
	"context"
	"time"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
)

// Direct ejecuta una Task contra UN modelo fijo, sin router (Fase 2) ni
// fusion/judgment. Quien construye Direct ya decidió el modelo — en
// Fase 1 eso es el usuario via --model; el router lo hará por su cuenta.
type Direct struct {
	executor *execution.Executor
	modelID  string
	fallback string
}

func NewDirect(executor *execution.Executor, modelID, fallback string) *Direct {
	return &Direct{executor: executor, modelID: modelID, fallback: fallback}
}

func (d *Direct) Name() core.StrategyName { return "direct" }

func (d *Direct) Run(ctx context.Context, task *core.Task) (*core.Result, error) {
	started := time.Now()

	req := core.Request{
		ModelID:  d.modelID,
		Prompt:   task.Prompt,
		Messages: task.Messages,
		Role:     core.RoleGenerator,
		Fallback: d.fallback,
	}
	if task.Constraints.MaxTokens > 0 {
		req.MaxTokens = task.Constraints.MaxTokens
	}

	resp, run, err := d.executor.Execute(ctx, req)

	return &core.Result{
		TaskID:    task.ID,
		Content:   resp.Content,
		Usage:     run.Usage,
		Strategy:  d.Name(),
		ModelRuns: []core.ModelRun{run},
		StartedAt: started,
		EndedAt:   time.Now(),
		Err:       err,
	}, err
}
