// Package memory define el contrato de memoria persistente de Arg0s
// (sección 12 de docs/ARG0S.md). MemoryStore es una interfaz -- SQLite
// (internal/memory/sqlite) es la implementación default; Engram queda
// como adapter opcional para una fase posterior, sin tocar esta
// interfaz (anti-objetivos de fase, ver CLAUDE.md).
package memory

import (
	"context"
	"time"
)

// Category son las siete categorías de sección 12/config.yaml
// (memory.categories). No hay categorías custom en Fase 3.
type Category string

const (
	CategoryDecision     Category = "DECISION"
	CategoryFact         Category = "FACT"
	CategoryBug          Category = "BUG"
	CategoryPreference   Category = "PREFERENCE"
	CategoryArchitecture Category = "ARCHITECTURE"
	CategoryTODO         Category = "TODO"
	CategoryLesson       Category = "LESSON"
)

// Memory es una entrada de memoria persistente (sección 12, modelo).
// Embedding queda sin usar en Fase 3 -- el ranking es lexical, no
// semántico (anti-objetivos: embeddings hasta que se decida lo
// contrario). El campo existe porque ya está en el schema (`memories.
// embedding BLOB`), pero ningún código lo llena ni lo lee todavía.
type Memory struct {
	ID           string
	Project      string
	Category     Category
	Title        string
	Content      string
	Confidence   float64
	Source       string // "user" | "run:184" | "judgment:led_001"
	Refs         []string
	CreatedAt    time.Time
	LastVerified time.Time
	Stale        bool
}

// MemoryQuery es el filtro + criterio de ranking de una consulta
// (sección 12, regla 1: "retrieval, no inyección" -- query → candidatos
// → ranking → filtro de relevancia → budget).
type MemoryQuery struct {
	Project      string
	Text         string // texto libre para el ranking lexical (ver Relevance en sqlite)
	Categories   []Category
	MinRelevance float64 // memory.min_relevance de config.yaml; 0 = sin piso
	Limit        int     // memory.max_fragments_per_query; 0 = sin límite
	IncludeStale bool    // false = las stale no aparecen en resultados normales
}

// MemoryStore es la interfaz de sección 5.4. Mínima a propósito -- si
// crece más de 3 métodos, revisar el diseño (regla del proyecto).
type MemoryStore interface {
	Store(ctx context.Context, m Memory) error
	Query(ctx context.Context, q MemoryQuery) ([]Memory, error)
	Forget(ctx context.Context, id string) error
}
