package fusion

import (
	"context"
	"sync"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
)

// Decision es cómo terminó una corrida de fusion -- sección 10.1.
type Decision string

const (
	DecisionPrimary     Decision = "primary"     // 0 llamadas extra, o divergencia baja
	DecisionSynthesized Decision = "synthesized" // divergencia media -- select_best
	DecisionEscalate    Decision = "escalate"    // divergencia alta -- señalado para Judgment Day (Fase 5), NO ejecutado acá
)

// Result es lo que Engine.Run devuelve.
type Result struct {
	Content    string
	Decision   Decision
	Divergence *Divergence // nil si no se corrieron secundarios (0 llamadas extra)
	ModelRuns  []core.ModelRun
	Escalate   bool
	// Winner es el índice de sección 10.3 (1 = primario, 2+ = secundario
	// N-1) cuando Decision==synthesized; 0 si no hubo síntesis. Sirve
	// para `arg0s bench fusion`: si el winner nunca es 1, es evidencia
	// de que el primario solo no alcanzaba -- ver bench.go.
	Winner int
}

// Engine implementa sección 10.1. New/Run nunca hablan con un provider
// directo -- todo pasa por execution.Executor (P4), igual que router y
// context compiler.
type Engine struct {
	exec        *execution.Executor
	cfg         config.FusionConfig
	synthesizer Synthesizer
}

func New(exec *execution.Executor, cfg config.FusionConfig, synthesizer Synthesizer) *Engine {
	return &Engine{exec: exec, cfg: cfg, synthesizer: synthesizer}
}

// Run corre el flujo adaptativo: primario primero, siempre. Escala a
// secundarios solo si uncertain (el router marcó incertidumbre alta,
// ej TaskProfile.Confidence bajo) O task.Profile.Complexity es high.
// Si ninguna de las dos aplica, o fusion está deshabilitado/no
// adaptativo, listo -- CERO llamadas extra (sección 10.1, el caso que
// hay que testear primero).
func (e *Engine) Run(ctx context.Context, task *core.Task, primaryModelID, fallback string, uncertain bool) (Result, error) {
	primaryReq := core.Request{
		ModelID: primaryModelID, Prompt: task.Prompt, Messages: task.Messages,
		Role: core.RoleGenerator, Fallback: fallback,
	}
	primaryResp, primaryRun, err := e.exec.Execute(ctx, primaryReq)
	if err != nil {
		return Result{}, err
	}
	runs := []core.ModelRun{primaryRun}

	if !e.shouldEscalate(task, uncertain) {
		return Result{Content: primaryResp.Content, Decision: DecisionPrimary, ModelRuns: runs}, nil
	}

	secondaryModels := e.secondaryModels(primaryModelID)
	if len(secondaryModels) == 0 {
		return Result{Content: primaryResp.Content, Decision: DecisionPrimary, ModelRuns: runs}, nil
	}

	results := e.runSecondaries(ctx, primaryReq, secondaryModels)

	// Lección de la carrera de streaming (Engram arg0s/lesson/stream-ctx-race,
	// internal/execution/stream.go:87-94): el for-range de runSecondaries
	// ya terminó (todas las goroutines mandaron su resultado, pase lo que
	// pase), pero eso NO dice si terminó porque todo corrió bien o
	// porque ctx se canceló a mitad de camino -- chequeo explícito acá,
	// no confiar en "no hubo panic" como señal de éxito.
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}

	var secondaryContents []string
	for _, r := range results {
		if r.err != nil {
			continue // un secundario que falló (no por cancelación, ya descartada arriba) simplemente no opina
		}
		runs = append(runs, r.run)
		secondaryContents = append(secondaryContents, r.resp.Content)
	}

	if len(secondaryContents) == 0 {
		// todos los secundarios fallaron -- no hay con qué medir
		// divergencia, el primario es lo único confiable que hay.
		return Result{Content: primaryResp.Content, Decision: DecisionPrimary, ModelRuns: runs}, nil
	}

	divergence := Measure(e.cfg.Divergence.Method, primaryResp.Content, secondaryContents)

	switch {
	case divergence.Score < e.cfg.Divergence.Threshold:
		return Result{Content: primaryResp.Content, Decision: DecisionPrimary, Divergence: &divergence, ModelRuns: runs}, nil

	case divergence.Score <= e.cfg.Divergence.EscalateThreshold:
		candidates := append([]string{primaryResp.Content}, secondaryContents...)
		synth, synthRun, err := e.synthesizer.Synthesize(ctx, task.Prompt, candidates)
		if err != nil {
			// el synthesizer falló -- degradar al primario es más seguro
			// que propagar un error por algo que ya tiene una respuesta
			// válida esperando (el primario).
			return Result{Content: primaryResp.Content, Decision: DecisionPrimary, Divergence: &divergence, ModelRuns: runs}, nil
		}
		runs = append(runs, synthRun)
		return Result{Content: synth.Content, Decision: DecisionSynthesized, Divergence: &divergence, ModelRuns: runs, Winner: synth.Winner}, nil

	default:
		// desacuerdo fuerte -- sección 10.1 dice "escalar a Judgment
		// Day". Judgment Day es Fase 5: acá solo se marca Escalate=true
		// en el resultado, nunca se ejecuta un judgment que no existe.
		return Result{Content: primaryResp.Content, Decision: DecisionEscalate, Divergence: &divergence, ModelRuns: runs, Escalate: true}, nil
	}
}

// shouldEscalate es el gate de sección 10.1: "¿el router marcó
// incertidumbre alta O complexity=high?". TaskProfile no tiene un
// campo "uncertain" propio -- uncertain lo decide el caller (la
// estrategia Fusion, ver orchestrator/strategies/fusion.go) a partir
// de TaskProfile.Confidence; acá solo se combina con Complexity y los
// flags de config.
func (e *Engine) shouldEscalate(task *core.Task, uncertain bool) bool {
	if !e.cfg.Enabled || !e.cfg.Adaptive {
		return false
	}
	return uncertain || task.Profile.Complexity == core.LevelHigh
}

func (e *Engine) secondaryModels(primaryModelID string) []string {
	max := e.cfg.MaxModels - 1 // -1 porque el primario ya cuenta como uno
	if max <= 0 {
		return nil
	}
	var out []string
	for _, m := range e.cfg.Models {
		if m == primaryModelID {
			continue
		}
		out = append(out, m)
		if len(out) >= max {
			break
		}
	}
	return out
}

type secondaryResult struct {
	run  core.ModelRun
	resp core.Response
	err  error
}

// runSecondaries corre modelos en paralelo con cancelación real: cada
// goroutine SIEMPRE manda su resultado al canal antes de terminar
// (incluso en error o si ctx ya se canceló) -- así el canal recibe
// exactamente len(models) mensajes sin importar qué pasó, y el
// for-range de abajo nunca se queda esperando algo que no va a llegar
// (esa es la clase de bug de la lección de streaming: un consumidor
// que asume que "el canal se cerró" == "todo terminó bien").
func (e *Engine) runSecondaries(ctx context.Context, primaryReq core.Request, models []string) []secondaryResult {
	out := make(chan secondaryResult, len(models))
	var wg sync.WaitGroup

	for _, modelID := range models {
		wg.Add(1)
		go func(modelID string) {
			defer wg.Done()
			req := primaryReq
			req.ModelID = modelID
			req.Fallback = "" // los secundarios no tienen fallback propio -- si fallan, simplemente no opinan
			resp, run, err := e.exec.Execute(ctx, req)
			out <- secondaryResult{run: run, resp: resp, err: err}
		}(modelID)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	results := make([]secondaryResult, 0, len(models))
	for r := range out {
		results = append(results, r)
	}
	return results
}
