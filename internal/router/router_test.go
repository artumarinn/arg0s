package router

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
)

func testModels() *config.ModelsFile {
	return &config.ModelsFile{
		Models: map[string]config.ModelConfig{
			"qwen-coder-7b":  {Provider: "ollama", ContextWindow: 32000, Local: true, Tier: "simple"},
			"gemini-flash":   {Provider: "gemini", ContextWindow: 1000000, Tier: "medium"},
			"qwen-coder-14b": {Provider: "ollama", ContextWindow: 32000, Local: true, Tier: "medium"},
			"gemini-pro":     {Provider: "gemini", ContextWindow: 2000000, Tier: "complex"},
		},
	}
}

func testTiers() map[string]config.TierConfig {
	return map[string]config.TierConfig{
		"simple":  {Models: []string{"qwen-coder-7b"}, MaxCostUSD: 0.0},
		"medium":  {Models: []string{"gemini-flash", "qwen-coder-14b"}, MaxCostUSD: 0.01},
		"complex": {Models: []string{"gemini-pro"}, MaxCostUSD: 0.10},
	}
}

type fakeCostSource struct {
	usedUSD float64
	err     error
}

func (f fakeCostSource) CostSinceUSD(ctx context.Context, since time.Time) (float64, error) {
	return f.usedUSD, f.err
}

func newTestRouter(t *testing.T, overrides []config.OverrideConfig, limits config.LimitsConfig, costs CostSource) *Router {
	t.Helper()
	cfg := config.RouterConfig{Mode: "heuristic", Tiers: testTiers(), Overrides: overrides}
	return New(cfg, limits, config.FusionConfig{}, testModels(), costs, nil, config.RoleConfig{})
}

func TestDecide_SimplePrompt_PicksSimpleTier(t *testing.T) {
	r := newTestRouter(t, nil, config.LimitsConfig{DailyCostUSD: 2, PerTaskCostUSD: 0.25}, nil)
	d, err := r.Decide(context.Background(), "t1", "cuánto es 2+2")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.SelectedModel != "qwen-coder-7b" {
		t.Fatalf("selected model = %q, want qwen-coder-7b", d.SelectedModel)
	}
	if d.Profile.Complexity != core.LevelLow {
		t.Fatalf("complexity = %q, want low", d.Profile.Complexity)
	}
}

func TestDecide_ArchitecturePrompt_PicksComplexTier(t *testing.T) {
	r := newTestRouter(t, nil, config.LimitsConfig{DailyCostUSD: 2, PerTaskCostUSD: 0.25}, nil)
	d, err := r.Decide(context.Background(), "t1", "diseñá la arquitectura de X")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.SelectedModel != "gemini-pro" {
		t.Fatalf("selected model = %q, want gemini-pro", d.SelectedModel)
	}
}

func TestDecide_FusionEnabledAndAdaptive_SelectsFusionOnHighComplexity(t *testing.T) {
	cfg := config.RouterConfig{Mode: "heuristic", Tiers: testTiers()}
	fusionCfg := config.FusionConfig{Enabled: true, Adaptive: true}
	r := New(cfg, config.LimitsConfig{DailyCostUSD: 2, PerTaskCostUSD: 0.25}, fusionCfg, testModels(), nil, nil, config.RoleConfig{})

	d, err := r.Decide(context.Background(), "t1", "diseñá la arquitectura de X")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.SelectedStrategy != "fusion" {
		t.Fatalf("selected strategy = %q, want fusion (complexity=high, fusion enabled+adaptive)", d.SelectedStrategy)
	}
}

func TestDecide_FusionEnabled_StaysDirectOnLowComplexity(t *testing.T) {
	cfg := config.RouterConfig{Mode: "heuristic", Tiers: testTiers()}
	fusionCfg := config.FusionConfig{Enabled: true, Adaptive: true}
	r := New(cfg, config.LimitsConfig{DailyCostUSD: 2, PerTaskCostUSD: 0.25}, fusionCfg, testModels(), nil, nil, config.RoleConfig{})

	d, err := r.Decide(context.Background(), "t1", "cuánto es 2+2")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.SelectedStrategy != "direct" {
		t.Fatalf("selected strategy = %q, want direct (complexity=low no justifica fusion)", d.SelectedStrategy)
	}
}

func TestDecide_ReviewHighOverride_ClampsForcedStrategyToDirect(t *testing.T) {
	overrides := []config.OverrideConfig{
		{When: config.OverrideWhen{Type: "review", Complexity: "high"}, ForceStrategy: "judgment"},
	}
	r := newTestRouter(t, overrides, config.LimitsConfig{DailyCostUSD: 2, PerTaskCostUSD: 0.25}, nil)

	d, err := r.Decide(context.Background(), "t1", "revisa este código y hacé una auditoría de seguridad exhaustiva del sistema completo")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.SelectedStrategy != "direct" {
		t.Fatalf("selected strategy = %q, want direct (Fase 2 no ejecuta judgment)", d.SelectedStrategy)
	}
	found := false
	for _, reason := range d.Reasons {
		if reason == "override pidió strategy=judgment, no implementado todavía -- ejecutando direct" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasons no explican el clamp de strategy: %v", d.Reasons)
	}
}

func TestDecide_OverrideForcesTierAndLocal(t *testing.T) {
	overrides := []config.OverrideConfig{
		{When: config.OverrideWhen{Privacy: "secret"}, ForceTier: "simple", ForceLocal: true},
	}
	r := newTestRouter(t, overrides, config.LimitsConfig{DailyCostUSD: 2, PerTaskCostUSD: 0.25}, nil)

	// classifyPrivacy nunca devuelve "secret" todavía (hook sin
	// implementar) -- se prueba el matching aplicando el override
	// directo contra un profile a mano.
	profile := core.TaskProfile{Complexity: core.LevelHigh, Privacy: core.PrivacySecret}
	ov := applyOverrides(profile, overrides)
	if ov.ForceTier != "simple" || !ov.ForceLocal {
		t.Fatalf("override no aplicó: %+v", ov)
	}
	_ = r
}

func TestSelectModel_RejectsInsufficientContextWindow(t *testing.T) {
	models := testModels()
	tiers := map[string]config.TierConfig{
		"medium": {Models: []string{"qwen-coder-14b"}, MaxCostUSD: 0.01},
	}
	res, err := selectModel("medium", tiers, models, 48000, false)
	if err == nil {
		t.Fatalf("expected error, got selection %+v", res)
	}
	if len(res.Rejected) != 1 {
		t.Fatalf("expected 1 rejected candidate, got %d", len(res.Rejected))
	}
	want := "context_window insuficiente (32k < 48k requeridos)"
	if res.Rejected[0].Reason != want {
		t.Fatalf("reason = %q, want %q", res.Rejected[0].Reason, want)
	}
}

// TestDecide_DailyBudgetAlmostGone_DegradesToAffordableTier es
// exactamente el escenario que pidió el checkpoint: un prompt que
// necesitaría escalar a complex, pero el presupuesto diario está casi
// agotado -- el router debe elegir el mejor tier que SÍ entra en
// presupuesto (medium, $0.01) en vez de fallar tras decidirse por el
// caro (complex, $0.10). Policy filtra ANTES de comprometerse al
// modelo final, sección 8.
func TestDecide_DailyBudgetAlmostGone_DegradesToAffordableTier(t *testing.T) {
	costs := fakeCostSource{usedUSD: 1.99} // quedan $0.01 de $2.00
	r := newTestRouter(t, nil, config.LimitsConfig{DailyCostUSD: 2.00, PerTaskCostUSD: 0.25}, costs)

	d, err := r.Decide(context.Background(), "t1", "diseñá la arquitectura de X")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.SelectedModel != "gemini-flash" {
		t.Fatalf("selected model = %q, want gemini-flash (degradado a tier medium, $0.01 entra en el resto del presupuesto)", d.SelectedModel)
	}
}

// TestDecide_DailyBudgetFullyExhausted_Blocks confirma que cuando NI
// SIQUIERA el tier más barato entra en lo que queda del presupuesto
// diario, el router bloquea con un mensaje claro -- no hay a dónde
// degradar.
func TestDecide_DailyBudgetFullyExhausted_Blocks(t *testing.T) {
	costs := fakeCostSource{usedUSD: 5.00} // ya se pasó del límite
	r := newTestRouter(t, nil, config.LimitsConfig{DailyCostUSD: 2.00, PerTaskCostUSD: 0.25}, costs)

	_, err := r.Decide(context.Background(), "t1", "diseñá la arquitectura de X")
	be, ok := err.(*BudgetExceededError)
	if !ok {
		t.Fatalf("expected *BudgetExceededError, got %T: %v", err, err)
	}
	if be.Limit != "daily" {
		t.Fatalf("limit = %q, want daily", be.Limit)
	}
}

// TestDecide_PerTaskCostLimitBlocks_WhenEveryTierExceedsIt usa un
// router con costo > 0 hasta en el tier simple -- si no, el tier
// gratuito siempre sería una vía de escape y nunca se vería un bloqueo
// real por per_task_cost_usd.
func TestDecide_PerTaskCostLimitBlocks_WhenEveryTierExceedsIt(t *testing.T) {
	cfg := config.RouterConfig{Mode: "heuristic", Tiers: map[string]config.TierConfig{
		"simple":  {Models: []string{"qwen-coder-7b"}, MaxCostUSD: 0.02},
		"medium":  {Models: []string{"gemini-flash"}, MaxCostUSD: 0.05},
		"complex": {Models: []string{"gemini-pro"}, MaxCostUSD: 0.10},
	}}
	r := New(cfg, config.LimitsConfig{DailyCostUSD: 2.00, PerTaskCostUSD: 0.01}, config.FusionConfig{}, testModels(), fakeCostSource{}, nil, config.RoleConfig{})

	_, err := r.Decide(context.Background(), "t1", "diseñá la arquitectura de X")
	be, ok := err.(*BudgetExceededError)
	if !ok {
		t.Fatalf("expected *BudgetExceededError, got %T: %v", err, err)
	}
	if be.Limit != "per_task" {
		t.Fatalf("limit = %q, want per_task", be.Limit)
	}
}

// TestDecide_ConcurrentRuns_DontBothFitOverDailyBudget: dos goroutines
// piden Decide al mismo tiempo, cada una estimando un costo que solo
// UNA de las dos puede pagar dentro de lo que queda del presupuesto
// diario. checkAndReserveBudget serializa check+reserva bajo r.mu --
// sin eso, las dos podrían leer "hay lugar" antes de que cualquiera
// reserve nada y pasar juntas por encima del límite.
func TestDecide_ConcurrentRuns_DontBothFitOverDailyBudget(t *testing.T) {
	costs := fakeCostSource{usedUSD: 0.95} // quedan $0.05 de $1.00
	cfg := config.RouterConfig{Mode: "heuristic", Tiers: map[string]config.TierConfig{
		"simple": {Models: []string{"qwen-coder-7b"}, MaxCostUSD: 0.03},
	}}
	r := New(cfg, config.LimitsConfig{DailyCostUSD: 1.00, PerTaskCostUSD: 1.00}, config.FusionConfig{}, testModels(), costs, nil, config.RoleConfig{})

	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := r.Decide(context.Background(), "t1", "cuánto es 2+2")
			results[i] = err
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	// $0.05 restantes, cada corrida pide $0.03: las dos juntas ($0.06)
	// no entran, pero una sola sí -- exactamente una debe pasar.
	if successes != 1 {
		t.Fatalf("expected exactly 1 success out of 2 concurrent runs (budget only fits one), got %d", successes)
	}
}
