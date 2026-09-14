package router

import (
	"context"
	"testing"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/mock"
)

// TODO(router/classifier): esto es el único test de punta a punta del
// modo classifier/hybrid -- corre contra el provider mock, nunca contra
// un modelo real. Antes de que router.mode pase a ser "hybrid" o
// "classifier" por default (hoy es "heuristic"), hace falta probarlo
// contra un modelo real y barato (ver CLAUDE.md / Engram
// arg0s/discovery/classifier-mode-untested-e2e).
func TestClassify_ParsesJSONFromMockProvider(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(mock.New(mock.Config{
		Name:     "mockprov",
		Response: `{"type": "refactor", "complexity": "high", "confidence": 0.92}`,
	}))
	models := &config.ModelsFile{Models: map[string]config.ModelConfig{
		"classifier-model": {Provider: "mockprov", ProviderModelID: "mockprov-1"},
	}}
	exec := execution.New(reg, models, map[string]execution.ProviderPolicy{}, nil)

	profile, err := classify(context.Background(), exec, config.RoleConfig{Model: "classifier-model"}, "refactoriza el módulo de auth")
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if profile.Type != core.TaskTypeRefactor {
		t.Fatalf("type = %q, want refactor", profile.Type)
	}
	if profile.Complexity != core.LevelHigh {
		t.Fatalf("complexity = %q, want high", profile.Complexity)
	}
	if profile.Confidence != 0.92 {
		t.Fatalf("confidence = %v, want 0.92", profile.Confidence)
	}
}

func TestClassify_InvalidJSON_ReturnsError(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(mock.New(mock.Config{Name: "mockprov", Response: "no soy JSON"}))
	models := &config.ModelsFile{Models: map[string]config.ModelConfig{
		"classifier-model": {Provider: "mockprov"},
	}}
	exec := execution.New(reg, models, map[string]execution.ProviderPolicy{}, nil)

	if _, err := classify(context.Background(), exec, config.RoleConfig{Model: "classifier-model"}, "hola"); err == nil {
		t.Fatal("esperaba error por JSON inválido, no hubo")
	}
}

// TestDecide_ClassifierMode_EndToEnd prueba el flujo completo prompt →
// Router (mode=classifier) → Executor → mock provider → TaskProfile
// parseado → tier seleccionado. Es el único test que ejercita
// router.Mode=="classifier" de punta a punta -- ver TODO arriba.
func TestDecide_ClassifierMode_EndToEnd(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(mock.New(mock.Config{
		Name:     "mockprov",
		Response: `{"type": "architecture", "complexity": "high", "confidence": 0.9}`,
	}))

	models := testModels()
	models.Models["classifier-model"] = config.ModelConfig{Provider: "mockprov", ProviderModelID: "mockprov-1"}
	exec := execution.New(reg, models, map[string]execution.ProviderPolicy{}, nil)

	cfg := config.RouterConfig{Mode: "classifier", Tiers: testTiers()}
	r := New(cfg, config.LimitsConfig{DailyCostUSD: 2, PerTaskCostUSD: 0.25}, config.FusionConfig{}, models, nil, exec, config.RoleConfig{Model: "classifier-model"})

	d, err := r.Decide(context.Background(), "t1", "cualquier prompt, lo decide el classifier mock")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.Mode != "classifier" {
		t.Fatalf("mode = %q, want classifier", d.Mode)
	}
	if d.SelectedModel != "gemini-pro" {
		t.Fatalf("selected model = %q, want gemini-pro (tier complex por classifier)", d.SelectedModel)
	}
}
