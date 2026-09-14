// Package contextc es el Context Compiler (sección 9): decide qué
// entra realmente al prompt bajo un presupuesto de tokens, a partir de
// Memory, Code Graph, Skills (stub, real en Fase 5) y archivos
// directos. "El grafo localiza, no reemplaza": nunca manda metadata
// del grafo en vez de código, y nunca manda un archivo completo si el
// grafo puede acotar el rango de líneas relevante.
package contextc

import "context"

// Query es lo que arma retrieval.go a partir del prompt -- Intent es
// el prompt normalizado, Files/Symbols son las entidades extraídas
// (regla de sección 9: "extraer Intent y entidades").
type Query struct {
	Intent  string
	Prompt  string
	Files   []string // paths o fragmentos de path mencionados
	Symbols []string // identificadores mencionados (CamelCase)
	Project string   // mismo valor que usa internal/codegraph -- la raíz del repo
}

// Fragment es sección 9 -- un pedazo de contexto de UNA fuente.
type Fragment struct {
	Source    string // "memory" | "codegraph" | "skill" | "file" | "artifact"
	Ref       string // "auth/token.go:40-95" | "ADR-014" | "skill:secure-go"
	Content   string
	Tokens    int
	Relevance float64
}

// ContextSource es la interfaz común de sección 5.4 -- memory/graph/
// files/skills se conectan de forma uniforme. budget es el techo de
// tokens que ESTA fuente puede gastar (ya aplicado budget_split); una
// fuente devuelve tantos Fragment como le entren, ordenados por
// relevancia -- budget.go decide qué sobrevive entre fuentes si hiciera
// falta recortar más.
type ContextSource interface {
	Name() string
	Retrieve(ctx context.Context, q Query, budget int) ([]Fragment, error)
}

// EstimateTokens es una aproximación (∼4 chars/token) -- Fase 3 no
// integra un tokenizer real. Se usa la MISMA función para medir el
// contexto compilado y el baseline en `arg0s bench context`, así que
// la comparación entre ambos es real (misma vara), aunque el número
// absoluto sea una aproximación de lo que un tokenizer de verdad
// contaría.
func EstimateTokens(s string) int {
	return (len(s) + 3) / 4
}
