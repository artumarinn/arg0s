package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
)

// classifierPrompt le pide al modelo classifier un TaskProfile como
// JSON estricto -- mismo espíritu que el esquema de finding de
// Judgment Day (sección 11.5): un contrato de salida chico y parseable.
const classifierPrompt = `Clasificá la siguiente tarea. Respondé SOLO un JSON con esta forma exacta, sin texto alrededor:
{"type": "qa|coding|refactor|review|architecture|research|creative", "complexity": "low|medium|high", "confidence": 0.0}

Tarea:
%s`

type classifierResponse struct {
	Type       string  `json:"type"`
	Complexity string  `json:"complexity"`
	Confidence float64 `json:"confidence"`
}

// classify llama al rol classifier vía el Executor (mismo componente
// que usa el resto del sistema -- router nunca habla con un provider
// directo, sección 8/P4) y parsea su salida. Si el modelo no devuelve
// JSON válido, no hay reintento acá (no es Judgment Day): se propaga
// el error y el caller decide si cae a heurística.
func classify(ctx context.Context, exec *execution.Executor, role config.RoleConfig, prompt string) (core.TaskProfile, error) {
	req := core.Request{
		ModelID:     role.Model,
		Prompt:      fmt.Sprintf(classifierPrompt, prompt),
		Role:        core.RoleClassifier,
		Fallback:    role.Fallback,
		Temperature: role.Temperature,
		MaxTokens:   role.MaxTokens,
	}

	resp, _, err := exec.Execute(ctx, req)
	if err != nil {
		return core.TaskProfile{}, fmt.Errorf("router: classifier falló: %w", err)
	}

	var parsed classifierResponse
	content := strings.TrimSpace(resp.Content)
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return core.TaskProfile{}, fmt.Errorf("router: classifier devolvió JSON inválido: %w", err)
	}

	return core.TaskProfile{
		Type:        core.TaskType(parsed.Type),
		Complexity:  core.Level(parsed.Complexity),
		Reasoning:   core.LevelMedium,
		ContextSize: core.LevelMedium,
		Latency:     core.PriorityMedium,
		CostBudget:  core.PriorityMedium,
		Confidence:  parsed.Confidence,
	}, nil
}
