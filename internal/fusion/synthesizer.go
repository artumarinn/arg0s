package fusion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
)

// SynthesisResult es sección 10.3 -- el synthesizer ELIGE una
// respuesta completa (select_best), no promedia texto. Content es la
// respuesta final a devolver (la del winner).
type SynthesisResult struct {
	Winner         int
	Reasoning      string
	BorrowedBlocks []string
	Content        string
}

// synthesisPrompt es casi textual a la sección 10.3 -- un contrato de
// salida chico y parseable, mismo espíritu que el esquema de finding
// de Judgment Day (sección 11.5).
const synthesisPrompt = `Recibís %d soluciones candidatas al mismo problema. No las mezcles. Evaluá cada una contra los criterios: corrección, manejo de errores, adherencia a las convenciones del proyecto, simplicidad. Elegí UNA como base. Si otra candidata tiene un bloque estrictamente superior, indicá cuál y por qué. Respondé SOLO este JSON, sin texto alrededor: {"winner": n, "reasoning": "...", "borrowed_blocks": ["..."]}

Tarea original:
%s

Candidatas:
%s`

// Synthesizer es la interfaz chica para poder testear fusion.go sin un
// Executor real -- ExecutorSummarizer de contextc es el mismo patrón.
type Synthesizer interface {
	Synthesize(ctx context.Context, prompt string, candidates []string) (SynthesisResult, core.ModelRun, error)
}

type ExecutorSynthesizer struct {
	Exec *execution.Executor
	Role config.RoleConfig
}

type synthesisResponse struct {
	Winner         int      `json:"winner"`
	Reasoning      string   `json:"reasoning"`
	BorrowedBlocks []string `json:"borrowed_blocks"`
}

func (s *ExecutorSynthesizer) Synthesize(ctx context.Context, prompt string, candidates []string) (SynthesisResult, core.ModelRun, error) {
	var b strings.Builder
	for i, c := range candidates {
		fmt.Fprintf(&b, "[%d]\n%s\n\n", i+1, c)
	}

	req := core.Request{
		ModelID: s.Role.Model, Role: core.RoleSynthesizer, Fallback: s.Role.Fallback, Temperature: s.Role.Temperature,
		Prompt: fmt.Sprintf(synthesisPrompt, len(candidates), prompt, b.String()),
	}
	resp, run, err := s.Exec.Execute(ctx, req)
	if err != nil {
		return SynthesisResult{}, run, fmt.Errorf("fusion: synthesizer: %w", err)
	}

	var parsed synthesisResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &parsed); err != nil {
		return SynthesisResult{}, run, fmt.Errorf("fusion: synthesizer devolvió JSON inválido: %w", err)
	}
	if parsed.Winner < 1 || parsed.Winner > len(candidates) {
		return SynthesisResult{}, run, fmt.Errorf("fusion: synthesizer eligió winner=%d fuera de rango (1..%d)", parsed.Winner, len(candidates))
	}

	return SynthesisResult{
		Winner: parsed.Winner, Reasoning: parsed.Reasoning, BorrowedBlocks: parsed.BorrowedBlocks,
		Content: candidates[parsed.Winner-1],
	}, run, nil
}
