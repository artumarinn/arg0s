package strategies

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/mock"
)

type testCatalog map[string]core.Model

func (c testCatalog) Resolve(id string) (core.Model, bool) {
	m, ok := c[id]
	return m, ok
}

func TestDirect_RunSuccess(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", Response: "un mutex es un candado"})
	registry := providers.NewRegistry()
	registry.Register(p)
	cat := testCatalog{"m1": {ID: "m1", Provider: "mock"}}
	exec := execution.New(registry, cat, map[string]execution.ProviderPolicy{"mock": {}}, nil)

	strat := NewDirect(exec, "m1", "")
	task := &core.Task{ID: "tsk_1", Prompt: "qué es un mutex"}

	result, err := strat.Run(context.Background(), task)

	require.NoError(t, err)
	require.Equal(t, "un mutex es un candado", result.Content)
	require.Equal(t, core.StrategyName("direct"), result.Strategy)
	require.Len(t, result.ModelRuns, 1)
	require.Equal(t, "tsk_1", string(result.TaskID))
	require.Positive(t, result.Usage.InputTokens)
}

func TestDirect_RunFailurePropagatesError(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", FailAlways: true})
	registry := providers.NewRegistry()
	registry.Register(p)
	cat := testCatalog{"m1": {ID: "m1", Provider: "mock"}}
	exec := execution.New(registry, cat, map[string]execution.ProviderPolicy{"mock": {}}, nil)

	strat := NewDirect(exec, "m1", "")
	task := &core.Task{ID: "tsk_2", Prompt: "hola"}

	result, err := strat.Run(context.Background(), task)

	require.Error(t, err)
	require.Equal(t, err, result.Err)
	require.Empty(t, result.Content)
}
