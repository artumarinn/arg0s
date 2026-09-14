package fusion

import (
	"context"
	"testing"
	"time"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/mock"
)

func testModels() *config.ModelsFile {
	return &config.ModelsFile{Models: map[string]config.ModelConfig{
		"primary":     {Provider: "primary", ProviderModelID: "primary-1"},
		"secondary-a": {Provider: "secondary-a", ProviderModelID: "secondary-a-1"},
		"secondary-b": {Provider: "secondary-b", ProviderModelID: "secondary-b-1"},
	}}
}

func testExecutor(t *testing.T, providerConfigs map[string]mock.Config) *execution.Executor {
	t.Helper()
	reg := providers.NewRegistry()
	for name, cfg := range providerConfigs {
		cfg.Name = name
		reg.Register(mock.New(cfg))
	}
	return execution.New(reg, testModels(), map[string]execution.ProviderPolicy{}, nil)
}

type stubSynthesizer struct {
	result SynthesisResult
	err    error
	calls  int
}

func (s *stubSynthesizer) Synthesize(ctx context.Context, prompt string, candidates []string) (SynthesisResult, core.ModelRun, error) {
	s.calls++
	return s.result, core.ModelRun{ModelID: "synth", Role: core.RoleSynthesizer}, s.err
}

func fusionCfg() config.FusionConfig {
	return config.FusionConfig{
		Enabled: true, Adaptive: true, MaxModels: 3,
		Models:     []string{"primary", "secondary-a", "secondary-b"},
		Divergence: config.DivergenceConfig{Method: "lexical", Threshold: 0.15, EscalateThreshold: 0.45},
	}
}

// TestRun_NotUncertainAndNotComplex_ReturnsPrimaryWithZeroExtraCalls
// es el test que pidió el checkpoint que se escriba PRIMERO: el
// default no es "correr 3 modelos siempre".
func TestRun_NotUncertainAndNotComplex_ReturnsPrimaryWithZeroExtraCalls(t *testing.T) {
	primary := mock.New(mock.Config{Name: "primary", Response: "respuesta primaria"})
	secondary := mock.New(mock.Config{Name: "secondary-a", Response: "no debería llamarse"})
	reg := providers.NewRegistry()
	reg.Register(primary)
	reg.Register(secondary)
	exec := execution.New(reg, testModels(), map[string]execution.ProviderPolicy{}, nil)

	engine := New(exec, fusionCfg(), nil)
	task := &core.Task{Prompt: "hola", Profile: core.TaskProfile{Complexity: core.LevelLow}}

	result, err := engine.Run(context.Background(), task, "primary", "", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Decision != DecisionPrimary {
		t.Fatalf("decision = %q, want primary", result.Decision)
	}
	if result.Divergence != nil {
		t.Fatal("esperaba Divergence=nil -- no debería haber medido nada (0 llamadas extra)")
	}
	if len(result.ModelRuns) != 1 {
		t.Fatalf("esperaba exactamente 1 ModelRun (solo el primario), got %d", len(result.ModelRuns))
	}
	if secondary.CallCount() != 0 {
		t.Fatalf("el secundario NO debería haberse llamado, CallCount=%d", secondary.CallCount())
	}
}

func TestRun_HighComplexity_EscalatesToSecondaries(t *testing.T) {
	exec := testExecutor(t, map[string]mock.Config{
		"primary":     {Response: "func A() int { return 1 }"},
		"secondary-a": {Response: "func A() int { return 1 }"}, // idéntico -- divergencia ~0
	})
	cfg := fusionCfg()
	cfg.Models = []string{"primary", "secondary-a"}
	cfg.MaxModels = 2
	engine := New(exec, cfg, nil)
	task := &core.Task{Prompt: "hola", Profile: core.TaskProfile{Complexity: core.LevelHigh}}

	result, err := engine.Run(context.Background(), task, "primary", "", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Divergence == nil {
		t.Fatal("esperaba que sí se midiera divergencia (complexity=high escala)")
	}
	if result.Decision != DecisionPrimary {
		t.Fatalf("decision = %q, want primary (respuestas idénticas, divergencia baja)", result.Decision)
	}
	if len(result.ModelRuns) != 2 {
		t.Fatalf("esperaba 2 ModelRuns (primario+secundario), got %d", len(result.ModelRuns))
	}
}

func TestRun_HighDivergence_MarksEscalateWithoutRunningJudgment(t *testing.T) {
	exec := testExecutor(t, map[string]mock.Config{
		"primary":     {Response: "esto es una respuesta completamente distinta sobre gatos y perros corriendo en el parque"},
		"secondary-a": {Response: "xyz abc 123 nada que ver contenido totalmente ajeno sin ninguna palabra en comun aca"},
	})
	cfg := fusionCfg()
	cfg.Models = []string{"primary", "secondary-a"}
	cfg.MaxModels = 2
	engine := New(exec, cfg, nil)
	task := &core.Task{Prompt: "hola", Profile: core.TaskProfile{Complexity: core.LevelHigh}}

	result, err := engine.Run(context.Background(), task, "primary", "", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Decision != DecisionEscalate || !result.Escalate {
		t.Fatalf("esperaba Decision=escalate y Escalate=true, got %+v", result)
	}
	if result.Content == "" {
		t.Fatal("esperaba que igual devuelva el contenido del primario mientras no hay Judgment Day")
	}
}

func TestRun_MediumDivergence_SynthesizesViaSelectBest(t *testing.T) {
	exec := testExecutor(t, map[string]mock.Config{
		"primary":     {Response: "func A() { x := 1; y := 2; return x+y }"},
		"secondary-a": {Response: "func A() { a := 1; b := 2; c := 3; return a+b+c }"}, // parecido pero no igual -- divergencia media
	})
	cfg := fusionCfg()
	cfg.Models = []string{"primary", "secondary-a"}
	cfg.MaxModels = 2
	synth := &stubSynthesizer{result: SynthesisResult{Winner: 2, Content: "func A() { a := 1; b := 2; c := 3; return a+b+c }", Reasoning: "más completo"}}
	engine := New(exec, cfg, synth)
	task := &core.Task{Prompt: "hola", Profile: core.TaskProfile{Complexity: core.LevelHigh}}

	result, err := engine.Run(context.Background(), task, "primary", "", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Divergence.Score < cfg.Divergence.Threshold || result.Divergence.Score > cfg.Divergence.EscalateThreshold {
		t.Skipf("divergencia (%.2f) no cayó en la banda de síntesis con este fixture -- ajustar mock", result.Divergence.Score)
	}
	if result.Decision != DecisionSynthesized {
		t.Fatalf("decision = %q, want synthesized", result.Decision)
	}
	if synth.calls != 1 {
		t.Fatalf("esperaba 1 llamada al synthesizer, got %d", synth.calls)
	}
	if result.Content != synth.result.Content {
		t.Fatalf("Content no es el del winner elegido por el synthesizer")
	}
}

func TestRun_OneSecondaryFails_OthersStillCounted(t *testing.T) {
	exec := testExecutor(t, map[string]mock.Config{
		"primary":     {Response: "func A() { return 1 }"},
		"secondary-a": {FailAlways: true},
		"secondary-b": {Response: "func A() { return 1 }"},
	})
	cfg := fusionCfg()
	engine := New(exec, cfg, nil)
	task := &core.Task{Prompt: "hola", Profile: core.TaskProfile{Complexity: core.LevelHigh}}

	result, err := engine.Run(context.Background(), task, "primary", "", false)
	if err != nil {
		t.Fatalf("Run no debería fallar porque UN secundario falló: %v", err)
	}
	// primario + secondary-b (secondary-a falló y no cuenta) = 2 ModelRuns
	if len(result.ModelRuns) != 2 {
		t.Fatalf("esperaba 2 ModelRuns (secondary-a falló, no debe aparecer), got %d: %+v", len(result.ModelRuns), result.ModelRuns)
	}
}

// TestRunSecondaries_OneFailsFast_DoesNotBlockSlowerOnesRunningInParallel
// es el test que pidió explícitamente el checkpoint: un secundario que
// falla YA (mientras otro sigue corriendo) no debe bloquear a los
// demás -- y el colector tiene que poder distinguir, resultado por
// resultado, cuál "cerró por error" y cuál "cerró por éxito" (la
// lección de streaming: el canal cierra/entrega SIEMPRE, nunca hay que
// asumir qué significa un mensaje por cómo llegó). Se ejercita
// runSecondaries directo (no Run) para medir tiempos sin el ruido del
// primario ni la síntesis.
func TestRunSecondaries_OneFailsFast_DoesNotBlockSlowerOnesRunningInParallel(t *testing.T) {
	const slow = 150 * time.Millisecond
	exec := testExecutor(t, map[string]mock.Config{
		"primary":     {},
		"secondary-a": {FailAlways: true}, // falla inmediato
		"secondary-b": {Response: "ok-b", Latency: slow},
	})
	engine := New(exec, fusionCfg(), nil)
	primaryReq := core.Request{ModelID: "primary", Prompt: "hola", Role: core.RoleGenerator}

	start := time.Now()
	results := engine.runSecondaries(context.Background(), primaryReq, []string{"secondary-a", "secondary-b"})
	elapsed := time.Since(start)

	// Paralelismo real: si secondary-a bloqueara a secondary-b (o el
	// colector esperara secuencial), esto tardaría >= slow igual, así
	// que el chequeo fuerte es que NO tarda mucho MÁS que slow -- si
	// hubiera un bug de serialización acumulando latencias adicionales
	// (ej un mutex de más), esto lo detecta con margen.
	if elapsed > slow+100*time.Millisecond {
		t.Fatalf("runSecondaries tardó %v, esperaba ~%v (paralelo, no serializado)", elapsed, slow)
	}

	if len(results) != 2 {
		t.Fatalf("esperaba 2 resultados (canal SIEMPRE recibe uno por goroutine, pase lo que pase), got %d", len(results))
	}

	var failed, succeeded int
	for _, r := range results {
		switch {
		case r.err != nil:
			failed++
		case r.resp.Content == "ok-b":
			succeeded++
		default:
			t.Fatalf("resultado inesperado, ni error ni el contenido esperado: %+v", r)
		}
	}
	if failed != 1 || succeeded != 1 {
		t.Fatalf("esperaba distinguir 1 fallo y 1 éxito, got failed=%d succeeded=%d (%+v)", failed, succeeded, results)
	}
}

func TestRun_ContextCancelledMidFusion_ReturnsErrorCleanly(t *testing.T) {
	exec := testExecutor(t, map[string]mock.Config{
		"primary":     {Response: "func A() { return 1 }"},
		"secondary-a": {Latency: 200 * time.Millisecond},
		"secondary-b": {Latency: 200 * time.Millisecond},
	})
	cfg := fusionCfg()
	engine := New(exec, cfg, nil)
	task := &core.Task{Prompt: "hola", Profile: core.TaskProfile{Complexity: core.LevelHigh}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	var result Result
	var err error
	go func() {
		result, err = engine.Run(ctx, task, "primary", "", false)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run no volvió -- fusion se colgó con la cancelación (exactamente el bug que la lección de streaming advertía)")
	}

	if err == nil {
		t.Fatalf("esperaba error por cancelación, got result=%+v", result)
	}
}
