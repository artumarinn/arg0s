package router

import (
	"fmt"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
)

// tierOrder es la escala simple → complex, usada tanto para mapear
// Complexity → tier como para escalar cuando un tier se queda sin
// candidatos viables.
var tierOrder = []string{"simple", "medium", "complex"}

// tierFor mapea TaskProfile.Complexity al tier base (sección 8). Un
// override con force_tier pisa este resultado antes de llegar acá.
func tierFor(complexity core.Level) string {
	switch complexity {
	case core.LevelHigh:
		return "complex"
	case core.LevelLow:
		return "simple"
	default:
		return "medium"
	}
}

// requiredContextWindow traduce TaskProfile.ContextSize a un mínimo de
// tokens exigido -- no hay Context Compiler todavía (Fase 3) que mida
// esto de verdad, así que son umbrales fijos documentados, suficientes
// para poder rechazar un modelo por "context_window insuficiente"
// (ejemplo de sección 8).
func requiredContextWindow(size core.Level) int {
	switch size {
	case core.LevelHigh:
		return 48000
	case core.LevelLow:
		return 4096
	default:
		return 16000
	}
}

// selectionResult es lo que scoring le devuelve al router para armar
// el RoutingDecision.
type selectionResult struct {
	Tier       string
	Model      string
	Candidates []string
	Rejected   []RejectedModel
	Reasons    []string
}

// tryTier evalúa SOLO los modelos de un tier (sin escalar a otro),
// filtrando por forceLocal y context_window. Es la unidad que tanto
// selectModel (escala hacia arriba por requisitos funcionales) como el
// downgrade por presupuesto (escala hacia abajo, ver router.go) usan
// para no duplicar el criterio de "qué hace viable a un modelo".
func tryTier(tierName string, tiers map[string]config.TierConfig, models *config.ModelsFile, requiredCtx int, forceLocal bool) (modelID string, rejected []RejectedModel, ok bool) {
	tier, exists := tiers[tierName]
	if !exists {
		return "", nil, false
	}
	for _, id := range tier.Models {
		model, resolved := models.Resolve(id)
		if !resolved {
			rejected = append(rejected, RejectedModel{Model: id, Reason: "no encontrado en el catálogo"})
			continue
		}
		if forceLocal && !model.Local {
			rejected = append(rejected, RejectedModel{Model: id, Reason: "no es local, requerido por policy de privacidad"})
			continue
		}
		if model.ContextWindow > 0 && model.ContextWindow < requiredCtx {
			rejected = append(rejected, RejectedModel{
				Model:  id,
				Reason: fmt.Sprintf("context_window insuficiente (%dk < %dk requeridos)", model.ContextWindow/1000, requiredCtx/1000),
			})
			continue
		}
		return id, rejected, true
	}
	return "", rejected, false
}

// selectModel recorre tierOrder empezando en startTier hacia arriba
// (más capaz, nunca más barato) hasta encontrar un modelo
// funcionalmente viable. Esto es solo el filtro FUNCIONAL (context
// window, disponibilidad, local) -- todavía no mira presupuesto; eso
// lo hace Router.selectAffordable después, sección 8 pide policy check
// antes de comprometerse a un modelo, no al revés.
func selectModel(startTier string, tiers map[string]config.TierConfig, models *config.ModelsFile, requiredCtx int, forceLocal bool) (selectionResult, error) {
	var res selectionResult
	start := indexOf(tierOrder, startTier)
	if start < 0 {
		start = 0
	}

	for _, tierName := range tierOrder[start:] {
		if _, ok := tiers[tierName]; !ok {
			continue
		}
		res.Tier = tierName
		res.Candidates = append(res.Candidates, tiers[tierName].Models...)

		modelID, rejected, ok := tryTier(tierName, tiers, models, requiredCtx, forceLocal)
		res.Rejected = append(res.Rejected, rejected...)
		if ok {
			res.Model = modelID
			res.Reasons = append(res.Reasons, fmt.Sprintf("tier=%s, modelo %s cumple los requisitos", tierName, modelID))
			return res, nil
		}

		if tierName != startTier {
			res.Reasons = append(res.Reasons, fmt.Sprintf("tier %s sin candidatos viables, escalando", tierName))
		}
	}

	return res, fmt.Errorf("router: ningún modelo disponible cumple los requisitos en ningún tier a partir de %q", startTier)
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
