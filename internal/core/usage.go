package core

// Usage registra consumo de tokens y costo de una llamada a un modelo.
type Usage struct {
	InputTokens  int
	OutputTokens int
	CachedTokens int
	CostUSD      float64
	// Estimated indica que los tokens se calcularon con heurística
	// (len(text)/4) en vez de venir del provider. Ver sección 7.
	Estimated bool
}
