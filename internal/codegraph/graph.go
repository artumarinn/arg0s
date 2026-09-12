// Package codegraph indexa símbolos y relaciones de un repo en SQLite
// vía un servidor LSP real (sección 13: LSP/SCIP, no tree-sitter
// manual -- cero CGO, regla dura #6). Fase 3 solo implementa gopls
// (Go) end-to-end; otros lenguajes quedan declarados en
// codegraph.servers pero sin cliente todavío (ver index.go).
package codegraph

import "time"

// Symbol es una fila de `symbols` (sección 13).
type Symbol struct {
	ID        string
	Project   string
	Name      string
	Kind      string // function|method|type|interface|var|const|package
	File      string // relativo a la raíz del proyecto
	LineStart int    // 1-indexado
	LineEnd   int
	Signature string
	Doc       string
	Language  string
	IndexedAt time.Time
}

// Edge es una fila de `edges`.
type Edge struct {
	FromSymbol string
	ToSymbol   string
	Kind       string // calls|implements|embeds|references|imports
	File       string
	Line       int
}

// File es lo que devuelven ImpactSet/RelatedFiles -- section 13 solo
// pide el path, no contenido (eso lo lee el Context Compiler después,
// "el grafo localiza, no reemplaza").
type File struct {
	Path string
}
