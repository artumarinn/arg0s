package contextc

import (
	"context"
	"fmt"

	"github.com/artumarinn/arg0s/internal/codegraph"
	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/memory"
)

// CompiledContext es la salida de sección 9 -- lo que realmente entra
// al prompt, bajo presupuesto, con Manifest de qué se incluyó/descartó
// y por qué.
type CompiledContext struct {
	Intent      string
	Fragments   []Fragment
	TotalTokens int
	Budget      int
	Manifest    Manifest
}

// Compiler orquesta retrieval → ranking → budget → (compresión) para
// las fuentes de sección 9. "graph" de budget_split queda reservado
// sin gastar en Fase 3: el contenido real que localiza el grafo se
// contabiliza en "code" (es código, no metadata de grafo) -- sección
// 13, "el grafo localiza, no reemplaza". Si más adelante se agrega un
// fragmento de puro resumen estructural (ej "callers de X: A, B, C"
// como texto, sin código), ESE consumiría "graph".
type Compiler struct {
	cfg        config.ContextConfig
	memory     ContextSource
	codegraph  ContextSource
	file       ContextSource
	skills     ContextSource
	summarizer Summarizer
}

func New(cfg config.ContextConfig, memoryStore memory.MemoryStore, minRelevance float64, graph *codegraph.Graph, summarizer Summarizer) *Compiler {
	return &Compiler{
		cfg:        cfg,
		memory:     &MemorySource{Store: memoryStore, MinRelevance: minRelevance},
		codegraph:  &CodeGraphSource{Graph: graph},
		file:       &FileSource{},
		skills:     &SkillsSource{},
		summarizer: summarizer,
	}
}

// Compile arma el CompiledContext completo para prompt. project es la
// raíz del repo -- mismo valor usado por `arg0s graph index` (así
// FindSymbol/RelatedFiles resuelven contra el índice correcto).
func (c *Compiler) Compile(ctx context.Context, prompt, project string) (CompiledContext, error) {
	q := NewQuery(prompt, project)
	budget := c.cfg.DefaultBudgetTokens

	promptFrag := Fragment{Source: "prompt", Ref: "prompt", Content: prompt, Tokens: EstimateTokens(prompt), Relevance: 1.0}
	included := []Fragment{promptFrag}
	var excluded []ManifestEntry

	memBudget := c.categoryBudget(budget, "memory")
	memFrags, err := c.memory.Retrieve(ctx, q, memBudget)
	if err != nil {
		return CompiledContext{}, err
	}
	memIncluded, memExcluded := applyBudget(ctx, "memory", memBudget, Rank(memFrags), c.summarizer)
	included = append(included, memIncluded...)
	excluded = append(excluded, memExcluded...)

	// "code" es compartido por CodeGraphSource (prioridad: rangos
	// acotados vía el grafo) y FileSource (archivos completos
	// mencionados a mano) -- se rankean juntos, no en dos budgets
	// separados, porque son la misma categoría de config.yaml.
	codeBudget := c.categoryBudget(budget, "code")
	graphFrags, err := c.codegraph.Retrieve(ctx, q, codeBudget)
	if err != nil {
		return CompiledContext{}, err
	}
	fileFrags, err := c.file.Retrieve(ctx, q, codeBudget)
	if err != nil {
		return CompiledContext{}, err
	}
	codeIncluded, codeExcluded := applyBudget(ctx, "code", codeBudget, Rank(append(graphFrags, fileFrags...)), c.summarizer)
	included = append(included, codeIncluded...)
	excluded = append(excluded, codeExcluded...)

	skillsBudget := c.categoryBudget(budget, "skills")
	skillFrags, err := c.skills.Retrieve(ctx, q, skillsBudget)
	if err != nil {
		return CompiledContext{}, err
	}
	skillIncluded, skillExcluded := applyBudget(ctx, "skills", skillsBudget, Rank(skillFrags), c.summarizer)
	included = append(included, skillIncluded...)
	excluded = append(excluded, skillExcluded...)

	manifest := Manifest{Excluded: excluded}
	for _, f := range included {
		manifest.Included = append(manifest.Included, ManifestEntry{Source: f.Source, Ref: f.Ref, Tokens: f.Tokens})
	}

	return CompiledContext{
		Intent: q.Intent, Fragments: included, TotalTokens: sumTokens(included),
		Budget: budget, Manifest: manifest,
	}, nil
}

func (c *Compiler) categoryBudget(total int, category string) int {
	return int(float64(total) * c.cfg.BudgetSplit[category])
}

// ExecutorSummarizer implementa Summarizer con el rol `summarizer` vía
// el Executor de Fase 1 -- Context Compiler nunca habla con un
// provider directo, sección 8/P4 aplica igual acá.
type ExecutorSummarizer struct {
	Exec *execution.Executor
	Role config.RoleConfig
}

func (s *ExecutorSummarizer) Compress(ctx context.Context, content string, targetTokens int) (string, error) {
	prompt := fmt.Sprintf(
		"Resumí el siguiente contenido preservando lo esencial. Apuntá a menos de %d tokens aproximadamente (~%d caracteres). Devolvé SOLO el resumen, sin comentario propio:\n\n%s",
		targetTokens, targetTokens*4, content)
	req := core.Request{
		ModelID: s.Role.Model, Prompt: prompt, Role: core.RoleSummarizer,
		Fallback: s.Role.Fallback, Temperature: s.Role.Temperature, MaxTokens: s.Role.MaxTokens,
	}
	resp, _, err := s.Exec.Execute(ctx, req)
	if err != nil {
		return "", fmt.Errorf("contextc: summarizer: %w", err)
	}
	return resp.Content, nil
}
