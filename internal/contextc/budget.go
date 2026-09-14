package contextc

import (
	"context"
	"fmt"
)

// ManifestEntry es una línea del manifiesto -- sección 9: "permite
// responder ¿por qué el modelo no vio ese archivo?".
type ManifestEntry struct {
	Source string
	Ref    string
	Tokens int
	Reason string
}

// Manifest es obligatorio en CompiledContext: qué entró y qué se
// descartó, con razón.
type Manifest struct {
	Included []ManifestEntry
	Excluded []ManifestEntry
}

// Summarizer comprime un fragmento demasiado grande -- lo implementa
// ExecutorSummarizer (compiler.go) usando el rol `summarizer` vía el
// Executor de Fase 1. Interfaz chica a propósito para poder testear
// budget.go sin un Executor real.
type Summarizer interface {
	Compress(ctx context.Context, content string, targetTokens int) (string, error)
}

// maxCompressionAttempts acota cuántas veces se intenta comprimir por
// categoría antes de rendirse y pasar a excluir -- sin este límite, un
// summarizer que no logra bajar de budget podría loopear para siempre.
const maxCompressionAttempts = 3

// applyBudget recorta candidates (YA ordenados por relevancia desc)
// para que no superen budget tokens. Antes de excluir directo, si hay
// summarizer, intenta comprimir el fragmento MÁS GRANDE (sección 9
// punto 12: "comprimir lo que exceda", no descartarlo de una).
// Agotados los intentos de compresión, cae a excluir desde el final
// (el menos relevante primero, ya que candidates viene ordenado).
func applyBudget(ctx context.Context, category string, budget int, candidates []Fragment, summarizer Summarizer) ([]Fragment, []ManifestEntry) {
	included := append([]Fragment(nil), candidates...)

	for attempt := 0; sumTokens(included) > budget && len(included) > 0 && summarizer != nil && attempt < maxCompressionAttempts; attempt++ {
		idx := largestIndex(included)
		compressed, err := summarizer.Compress(ctx, included[idx].Content, budget)
		if err != nil || compressed == "" || len(compressed) >= len(included[idx].Content) {
			break // el summarizer no ayudó -- pasa al fallback de exclusión
		}
		included[idx].Content = compressed
		included[idx].Tokens = EstimateTokens(compressed)
		included[idx].Ref += " (comprimido)"
	}

	var excluded []ManifestEntry
	for sumTokens(included) > budget && len(included) > 0 {
		last := included[len(included)-1]
		excluded = append(excluded, ManifestEntry{
			Source: last.Source, Ref: last.Ref, Tokens: last.Tokens,
			Reason: fmt.Sprintf("excede el budget de %q (%d tokens disponibles)", category, budget),
		})
		included = included[:len(included)-1]
	}
	return included, excluded
}

func sumTokens(frags []Fragment) int {
	total := 0
	for _, f := range frags {
		total += f.Tokens
	}
	return total
}

func largestIndex(frags []Fragment) int {
	idx := 0
	for i, f := range frags {
		if f.Tokens > frags[idx].Tokens {
			idx = i
		}
	}
	return idx
}
