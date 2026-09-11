package execution

import "github.com/artumarinn/arg0s/internal/core"

// accountUsage calcula el costo de una llamada exitosa. Si el provider
// no reportó tokens (Usage zero value), estima con la heurística
// len(text)/4 y marca Estimated=true (sección 7 — Fase 1: heurística;
// tokenizer real queda para cuando haga falta precisión).
func accountUsage(reported core.Usage, model core.Model, req core.Request, resp core.Response) core.Usage {
	u := reported
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		u.InputTokens = estimateTokens(req.Prompt)
		u.OutputTokens = estimateTokens(resp.Content)
		u.Estimated = true
	}
	u.CostUSD = tokenCost(u.InputTokens, model.CostInputPer1M) + tokenCost(u.OutputTokens, model.CostOutputPer1M)
	return u
}

// estimateFailureUsage contabiliza el input aunque la llamada haya
// fallado — esos tokens ya se gastaron (sección 7, regla de accounting).
func estimateFailureUsage(req core.Request) core.Usage {
	return core.Usage{InputTokens: estimateTokens(req.Prompt), Estimated: true}
}

// accountStreamUsage cierra el accounting de un stream que terminó bien.
// content es todo lo acumulado de los Delta recibidos.
func accountStreamUsage(finalUsage *core.Usage, model core.Model, req core.Request, content string) core.Usage {
	var u core.Usage
	if finalUsage != nil {
		u = *finalUsage
	} else {
		u = core.Usage{InputTokens: estimateTokens(req.Prompt), OutputTokens: estimateTokens(content), Estimated: true}
	}
	u.CostUSD = tokenCost(u.InputTokens, model.CostInputPer1M) + tokenCost(u.OutputTokens, model.CostOutputPer1M)
	return u
}

// partialStreamUsage contabiliza lo que se alcanzó a generar cuando un
// stream muere a mitad de camino (error del provider o cancelación) —
// esos tokens ya se generaron, se cobran igual.
func partialStreamUsage(req core.Request, content string, finalUsage *core.Usage) core.Usage {
	if finalUsage != nil {
		return *finalUsage
	}
	return core.Usage{InputTokens: estimateTokens(req.Prompt), OutputTokens: estimateTokens(content), Estimated: true}
}

func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	n := len(s) / 4
	if n == 0 {
		n = 1
	}
	return n
}

func tokenCost(tokens int, usdPer1M float64) float64 {
	return float64(tokens) / 1_000_000 * usdPer1M
}
