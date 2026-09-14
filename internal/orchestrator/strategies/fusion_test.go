package strategies

import (
	"context"
	"testing"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/fusion"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/mock"
)

func TestFusion_Run_AggregatesUsageAcrossModelRuns(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(mock.New(mock.Config{Name: "primary", Response: "respuesta"}))
	models := &config.ModelsFile{Models: map[string]config.ModelConfig{
		"primary": {Provider: "primary", Cost: config.ModelCost{InputPer1MUSD: 1, OutputPer1MUSD: 1}},
	}}
	exec := execution.New(reg, models, map[string]execution.ProviderPolicy{}, nil)
	engine := fusion.New(exec, config.FusionConfig{Enabled: true, Adaptive: true, MaxModels: 1}, nil)

	strat := NewFusion(engine, "primary", "")
	task := &core.Task{Prompt: "hola", Profile: core.TaskProfile{Complexity: core.LevelLow}}

	result, err := strat.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Strategy != "fusion" {
		t.Fatalf("Strategy = %q, want fusion", result.Strategy)
	}
	if len(result.ModelRuns) != 1 {
		t.Fatalf("esperaba 1 ModelRun (no escaló), got %d", len(result.ModelRuns))
	}
	if result.Usage.CostUSD < 0 {
		t.Fatalf("Usage no se agregó correctamente: %+v", result.Usage)
	}
}

func TestFusion_UncertaintyThreshold_ZeroConfidenceIsNotUncertain(t *testing.T) {
	// Confidence==0 (TaskProfile nunca perfilado, ej --model manual)
	// NO debe contarse como "incertidumbre alta" -- si no, fusion
	// escalaría siempre que no haya pasado por el router.
	profile := core.TaskProfile{Complexity: core.LevelLow, Confidence: 0}
	uncertain := profile.Confidence > 0 && profile.Confidence < uncertaintyThreshold
	if uncertain {
		t.Fatal("Confidence=0 no debería contar como incertidumbre alta")
	}
}
