package router

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
)

// Router implementa el flujo de sección 8: perfilar → overrides →
// policy check → tier → modelo → estrategia -- en ESE orden: el
// presupuesto filtra candidatos antes de comprometerse a uno, nunca
// después (ver selectAffordable). Fusion y Judgment no existen todavía
// (Fase 4/5) -- SelectedStrategy siempre resuelve a "direct" en Fase
// 2; un override que pida otra cosa queda anotado en Reasons pero no
// se ejecuta.
type Router struct {
	cfg    config.RouterConfig
	limits config.LimitsConfig
	models *config.ModelsFile
	costs  CostSource

	// classifierExec/classifierRole son nil-safe: solo se usan si
	// cfg.Mode es "classifier" o "hybrid". En modo heuristic (default)
	// el router nunca llama a un modelo para perfilar.
	classifierExec *execution.Executor
	classifierRole config.RoleConfig

	// mu protege reservedUSD/reservedDay -- ver checkAndReserveBudget en
	// policy.go. Solo serializa Decide() dentro de ESTE proceso (varias
	// goroutines contra el mismo *Router); no protege contra dos
	// procesos `arg0s run` separados (eso es Fase 4, con daemon).
	mu          sync.Mutex
	reservedUSD float64
	reservedDay time.Time
}

func New(cfg config.RouterConfig, limits config.LimitsConfig, models *config.ModelsFile, costs CostSource, classifierExec *execution.Executor, classifierRole config.RoleConfig) *Router {
	return &Router{
		cfg: cfg, limits: limits, models: models, costs: costs,
		classifierExec: classifierExec, classifierRole: classifierRole,
	}
}

// Decide perfila prompt, aplica overrides y policy, y devuelve la
// RoutingDecision completa. No ejecuta nada -- eso lo hace el caller
// (arg0s run) o no lo hace (arg0s route explain).
func (r *Router) Decide(ctx context.Context, taskID, prompt string) (RoutingDecision, error) {
	cwd, _ := os.Getwd()
	profile, mode := r.profile(ctx, prompt, cwd)

	ov := applyOverrides(profile, r.cfg.Overrides)

	startTier := tierFor(profile.Complexity)
	if ov.ForceTier != "" {
		startTier = ov.ForceTier
	}

	decision := RoutingDecision{
		TaskID:               taskID,
		Profile:              summarize(profile),
		Mode:                 mode,
		ClassifierConfidence: profile.Confidence,
		SelectedStrategy:     "direct",
		Reasons:              ov.Reasons,
	}

	requiredCtx := requiredContextWindow(profile.ContextSize)

	var sel selectionResult
	var err error
	if ov.ForceModel != "" {
		sel, err = r.forcedModel(ov.ForceModel, ov.ForceLocal)
		if err == nil {
			err = r.checkAndReserveBudget(ctx, r.cfg.Tiers[sel.Tier].MaxCostUSD)
		}
	} else {
		sel, err = r.selectAffordable(ctx, startTier, requiredCtx, ov.ForceLocal)
	}
	if err != nil {
		return RoutingDecision{}, err
	}

	decision.Candidates = sel.Candidates
	decision.Rejected = sel.Rejected
	decision.SelectedModel = sel.Model
	decision.Reasons = append(decision.Reasons, sel.Reasons...)

	if ov.ForceStrategy != "" && ov.ForceStrategy != "direct" {
		decision.Reasons = append(decision.Reasons,
			fmt.Sprintf("override pidió strategy=%s, pero Fase 2 solo ejecuta direct -- ejecutando direct", ov.ForceStrategy))
	}

	return decision, nil
}

// selectAffordable filtra por presupuesto ANTES de comprometerse a un
// modelo (sección 8: policy check es el paso [3], antes de selección
// de tier/modelo [4]/[5]). Primero hace la pasada funcional
// (selectModel, que solo escala hacia tiers MÁS caros si hace falta
// por context_window). Si ese resultado no entra en presupuesto,
// busca -- de ese tier hacia abajo -- el tier más barato que sea
// funcionalmente viable Y entre en presupuesto, en vez de fallar
// directo con el tier caro ya decidido. limits.on_exceed decide qué
// tan lejos llega esa búsqueda:
//   - "warn": no busca nada más barato, ejecuta el tier ideal igual y
//     deja el aviso en Reasons.
//   - "block" (default): busca el tier más capaz que entre en
//     presupuesto; si ninguno entra, bloquea con BudgetExceededError.
//   - "downgrade_to_local": igual que block, pero exige local en la
//     búsqueda de downgrade (fuerza el modelo más barato posible).
func (r *Router) selectAffordable(ctx context.Context, startTier string, requiredCtx int, forceLocal bool) (selectionResult, error) {
	sel, err := selectModel(startTier, r.cfg.Tiers, r.models, requiredCtx, forceLocal)
	if err != nil {
		return sel, err
	}

	budgetErr := r.checkAndReserveBudget(ctx, r.cfg.Tiers[sel.Tier].MaxCostUSD)
	if budgetErr == nil {
		return sel, nil
	}

	if r.limits.OnExceed == "warn" {
		sel.Reasons = append(sel.Reasons, fmt.Sprintf("%v (on_exceed=warn, se ejecuta igual)", budgetErr))
		return sel, nil
	}

	downgradeToLocal := r.limits.OnExceed == "downgrade_to_local"
	idealIdx := indexOf(tierOrder, sel.Tier)
	for i := idealIdx - 1; i >= 0; i-- {
		tierName := tierOrder[i]
		modelID, rejected, ok := tryTier(tierName, r.cfg.Tiers, r.models, requiredCtx, forceLocal || downgradeToLocal)
		if !ok {
			sel.Rejected = append(sel.Rejected, rejected...)
			continue
		}
		if err := r.checkAndReserveBudget(ctx, r.cfg.Tiers[tierName].MaxCostUSD); err != nil {
			sel.Rejected = append(sel.Rejected, RejectedModel{Model: modelID, Reason: err.Error()})
			continue
		}
		sel.Tier = tierName
		sel.Model = modelID
		sel.Rejected = append(sel.Rejected, rejected...)
		sel.Reasons = append(sel.Reasons,
			fmt.Sprintf("tier %s excede presupuesto, se degrada a tier %s (%s) por limits.on_exceed=%s", tierOrder[idealIdx], tierName, modelID, r.limits.OnExceed))
		return sel, nil
	}

	return sel, budgetErr
}

func (r *Router) forcedModel(modelID string, forceLocal bool) (selectionResult, error) {
	model, ok := r.models.Resolve(modelID)
	if !ok {
		return selectionResult{}, fmt.Errorf("router: force_model %q no encontrado en el catálogo", modelID)
	}
	if forceLocal && !model.Local {
		return selectionResult{}, fmt.Errorf("router: force_model %q no es local, pero la policy exige local", modelID)
	}
	return selectionResult{
		Tier:       model.Tier,
		Model:      modelID,
		Candidates: []string{modelID},
		Reasons:    []string{fmt.Sprintf("modelo %s forzado por override", modelID)},
	}, nil
}

// profile perfila según cfg.Mode. heuristic (default) nunca llama a un
// modelo. classifier siempre lo llama. hybrid arranca con heurística y
// solo escala a classifier si Confidence < classifier_threshold.
func (r *Router) profile(ctx context.Context, prompt, cwd string) (core.TaskProfile, string) {
	heuristic := ProfileHeuristic(prompt, cwd)

	switch r.cfg.Mode {
	case "classifier":
		return r.withClassifier(ctx, prompt, heuristic), "classifier"
	case "hybrid":
		if heuristic.Confidence < r.cfg.ClassifierThreshold {
			return r.withClassifier(ctx, prompt, heuristic), "hybrid"
		}
		return heuristic, "hybrid"
	default:
		return heuristic, "heuristic"
	}
}

// withClassifier llama al modelo classifier y sobre-escribe
// Type/Complexity/Confidence del perfil heurístico. Privacy y el resto
// de los defaults se mantienen: la privacidad es una señal de
// filesystem/git, no una decisión de un LLM. Si el classifier falla o
// no está configurado (Executor nil), se degrada a la heurística sin
// cortar el flujo -- un classifier caído no debe romper `arg0s run`.
func (r *Router) withClassifier(ctx context.Context, prompt string, heuristic core.TaskProfile) core.TaskProfile {
	if r.classifierExec == nil || r.classifierRole.Model == "" {
		return heuristic
	}
	classified, err := classify(ctx, r.classifierExec, r.classifierRole, prompt)
	if err != nil {
		return heuristic
	}
	heuristic.Type = classified.Type
	heuristic.Complexity = classified.Complexity
	heuristic.Confidence = classified.Confidence
	return heuristic
}
