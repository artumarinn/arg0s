package core

// Model es metadata de un modelo: capacidades, ventana, costo.
// Distinto de Provider (endpoint que lo ejecuta) y Runtime (agente con
// loop propio) — ver docs/ARG0S.md sección 4.2.
type Model struct {
	ID              string // alias estable, ej "gemini-flash"
	Provider        string // nombre del provider, ej "gemini"
	ProviderModelID string // id real ante el provider, ej "gemini-2.5-flash"
	ContextWindow   int
	MaxOutputTokens int
	CostInputPer1M  float64
	CostOutputPer1M float64
	Capabilities    []string
	Strengths       []string
	Tier            string // simple | medium | complex
	Local           bool
}
