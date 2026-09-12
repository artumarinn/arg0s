package codegraph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/artumarinn/arg0s/internal/codegraph/lsp"
)

// Stats es lo que reporta `arg0s graph index` -- tiene que ser
// explícito sobre qué se indexó de verdad y qué se saltó, nunca fallar
// en silencio (requisito 9 de Fase 3).
type Stats struct {
	FilesIndexed   int
	SymbolsIndexed int
	EdgesIndexed   int
	// SkippedLanguages son extensiones encontradas sin LSP implementado
	// -- Fase 3 solo tiene cliente real para Go (gopls). No se indexan
	// con "confidence: low" heurístico todavía (eso sería inventar un
	// segundo indexador por lenguaje sin que nadie lo haya pedido);
	// simplemente se listan acá para que el usuario sepa que existen y
	// no fueron tocados.
	SkippedLanguages map[string]int
}

// lspRange/lspPosition/documentSymbol/location son el subset de tipos
// LSP que necesitamos parsear -- no un SDK completo.
type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail"`
	Kind           int              `json:"kind"`
	Range          lspRange         `json:"range"`
	SelectionRange lspRange         `json:"selectionRange"`
	Children       []documentSymbol `json:"children"`
}

type location struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

// symbolKindNames mapea los SymbolKind numéricos del protocolo LSP
// (spec 3.17, sección "SymbolKind") a las categorías de sección 13.
// Los kinds no listados (namespace, property, string literal, etc) se
// ignoran -- no son la unidad de granularidad que router/context
// compiler necesitan.
var symbolKindNames = map[int]string{
	4:  "package",   // Package
	5:  "type",      // Class
	6:  "method",    // Method
	9:  "method",    // Constructor
	10: "type",      // Enum
	11: "interface", // Interface
	12: "function",  // Function
	13: "var",       // Variable
	14: "const",     // Constant
	23: "type",      // Struct
}

// Index recorre rootDir, indexa los .go con gopls (único lenguaje real
// de Fase 3) y arma `symbols`/`edges` para project. Devuelve Stats
// aunque falle indexar algún archivo puntual -- un archivo roto no
// tira abajo la corrida completa, pero SÍ queda en el error si gopls
// ni siquiera pudo arrancar (eso sí es fatal: sin LSP no hay índice).
func Index(ctx context.Context, goplsCmd string, goplsArgs []string, rootDir, project string, db *sql.DB, ignore []string) (Stats, error) {
	stats := Stats{SkippedLanguages: map[string]int{}}

	goFiles, otherExts, err := walkSourceFiles(rootDir, ignore)
	if err != nil {
		return stats, fmt.Errorf("codegraph: walk %s: %w", rootDir, err)
	}
	stats.SkippedLanguages = otherExts

	if len(goFiles) == 0 {
		return stats, nil
	}

	if _, err := exec.LookPath(goplsCmd); err != nil {
		return stats, fmt.Errorf("codegraph: %q no está en el PATH -- instalalo (go install golang.org/x/tools/gopls@latest) para indexar Go; sin LSP no hay índice real para este lenguaje (sección 13, no hay fallback regex implementado)", goplsCmd)
	}

	client, err := lsp.Start(goplsCmd, goplsArgs, rootDir)
	if err != nil {
		return stats, fmt.Errorf("codegraph: arrancar %s: %w", goplsCmd, err)
	}
	defer client.Close(ctx)

	rootURI := "file://" + rootDir
	if err := client.Initialize(ctx, rootURI); err != nil {
		return stats, fmt.Errorf("codegraph: %w", err)
	}

	symbolsByFile := map[string][]Symbol{}   // relFile -> symbols (para la pasada de edges)
	selectionPos := map[string]lspPosition{} // symbol ID -> posición del identificador (para textDocument/references)
	now := time.Now()

	for _, absPath := range goFiles {
		relPath, _ := filepath.Rel(rootDir, absPath)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue // archivo puntual ilegible -- no tira la corrida
		}
		uri := "file://" + absPath
		if err := client.Notify("textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{"uri": uri, "languageId": "go", "version": 1, "text": string(content)},
		}); err != nil {
			continue
		}

		raw, err := client.Call(ctx, "textDocument/documentSymbol", map[string]any{
			"textDocument": map[string]any{"uri": uri},
		})
		if err != nil {
			continue // gopls no pudo analizar este archivo puntual -- se salta, no aborta todo
		}
		var docSymbols []documentSymbol
		if err := json.Unmarshal(raw, &docSymbols); err != nil {
			continue
		}

		symbols, positions := flattenSymbols(docSymbols, project, relPath, now)
		symbolsByFile[relPath] = symbols
		for id, pos := range positions {
			selectionPos[id] = pos
		}
		stats.FilesIndexed++
	}

	if err := replaceSymbols(ctx, db, project, symbolsByFile); err != nil {
		return stats, err
	}
	for _, syms := range symbolsByFile {
		stats.SymbolsIndexed += len(syms)
	}

	edges, err := indexEdges(ctx, client, rootDir, symbolsByFile, selectionPos)
	if err != nil {
		return stats, err
	}
	if err := replaceEdges(ctx, db, symbolsByFile, edges); err != nil {
		return stats, err
	}
	stats.EdgesIndexed = len(edges)

	return stats, nil
}

// flattenSymbols aplana el árbol jerárquico de documentSymbol (una
// función puede tener variables locales como children que no nos
// interesan) en la lista plana que persiste `symbols` -- solo los
// kinds mapeados en symbolKindNames.
func flattenSymbols(docSymbols []documentSymbol, project, file string, now time.Time) ([]Symbol, map[string]lspPosition) {
	var out []Symbol
	positions := map[string]lspPosition{}
	var walk func(ds documentSymbol)
	walk = func(ds documentSymbol) {
		if kind, ok := symbolKindNames[ds.Kind]; ok {
			id := symbolID(project, file, ds.Range.Start.Line+1, ds.Name)
			out = append(out, Symbol{
				ID:        id,
				Project:   project,
				Name:      ds.Name,
				Kind:      kind,
				File:      file,
				LineStart: ds.Range.Start.Line + 1,
				LineEnd:   ds.Range.End.Line + 1,
				Signature: ds.Detail,
				Language:  "go",
				IndexedAt: now,
			})
			// SelectionRange es la posición del IDENTIFICADOR (el nombre
			// "Execute" dentro de "func (e *Executor) Execute(...)"), no
			// el inicio de la declaración (que arranca en "func") --
			// textDocument/references necesita el cursor parado sobre el
			// identificador, si no gopls no resuelve ningún símbolo.
			positions[id] = ds.SelectionRange.Start
		}
		// Los children de un método/función (variables locales) no se
		// indexan como symbols propios, pero SÍ hay que recorrer los de
		// un type/interface (sus métodos y campos) -- documentSymbol de
		// gopls anida ambos casos igual, así que se camina siempre; el
		// filtro de kind de arriba ya descarta lo que no nos interesa.
		for _, child := range ds.Children {
			walk(child)
		}
	}
	for _, ds := range docSymbols {
		walk(ds)
	}
	return out, positions
}

func symbolID(project, file string, lineStart int, name string) string {
	return fmt.Sprintf("%s::%s::%d::%s", project, file, lineStart, name)
}

// replaceSymbols borra los symbols previos de cada archivo tocado y
// escribe los nuevos -- reindexar un archivo no debe dejar symbols
// fantasma de una versión vieja.
func replaceSymbols(ctx context.Context, db *sql.DB, project string, byFile map[string][]Symbol) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("codegraph: begin tx: %w", err)
	}
	defer tx.Rollback()

	for file, symbols := range byFile {
		if _, err := tx.ExecContext(ctx, `DELETE FROM symbols WHERE project = ? AND file = ?`, project, file); err != nil {
			return fmt.Errorf("codegraph: delete old symbols for %s: %w", file, err)
		}
		for _, s := range symbols {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO symbols (id, project, name, kind, file, line_start, line_end, signature, doc, language, indexed_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				s.ID, s.Project, s.Name, s.Kind, s.File, s.LineStart, s.LineEnd, s.Signature, s.Doc, s.Language, s.IndexedAt.Unix(),
			); err != nil {
				return fmt.Errorf("codegraph: insert symbol %s: %w", s.ID, err)
			}
		}
	}
	return tx.Commit()
}

// indexEdges usa textDocument/references (no call hierarchy) sobre
// cada symbol de kind function/method: por cada referencia devuelta
// (excluyendo la propia declaración), busca qué symbol indexado
// contiene esa línea en ese archivo -- ESE es el caller. references es
// más simple de consumir que callHierarchy (una lista plana en vez de
// un árbol) y lo soportan todos los LSP mainstream igual.
func indexEdges(ctx context.Context, client *lsp.Client, rootDir string, symbolsByFile map[string][]Symbol, selectionPos map[string]lspPosition) ([]Edge, error) {
	var edges []Edge
	for file, symbols := range symbolsByFile {
		for _, target := range symbols {
			if target.Kind != "function" && target.Kind != "method" {
				continue
			}
			pos, ok := selectionPos[target.ID]
			if !ok {
				continue
			}
			raw, err := client.Call(ctx, "textDocument/references", map[string]any{
				"textDocument": map[string]any{"uri": "file://" + filepath.Join(rootDir, file)},
				"position":     pos,
				"context":      map[string]any{"includeDeclaration": false},
			})
			if err != nil {
				continue // símbolo puntual sin referencias resolubles -- no aborta el resto
			}
			var locs []location
			if err := json.Unmarshal(raw, &locs); err != nil {
				continue
			}
			for _, loc := range locs {
				refFile, ok := relFromURI(rootDir, loc.URI)
				if !ok {
					continue
				}
				caller := symbolAtLine(symbolsByFile[refFile], loc.Range.Start.Line+1)
				if caller == nil || caller.ID == target.ID {
					continue
				}
				edges = append(edges, Edge{FromSymbol: caller.ID, ToSymbol: target.ID, Kind: "calls", File: refFile, Line: loc.Range.Start.Line + 1})
			}
		}
	}
	return edges, nil
}

func relFromURI(rootDir, uri string) (string, bool) {
	path := strings.TrimPrefix(uri, "file://")
	rel, err := filepath.Rel(rootDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return rel, true
}

// symbolAtLine devuelve el symbol de función/método más específico
// (rango más chico) que contiene line -- una referencia dentro del
// cuerpo de una función pertenece a esa función, no al archivo entero.
func symbolAtLine(symbols []Symbol, line int) *Symbol {
	var best *Symbol
	for i := range symbols {
		s := &symbols[i]
		if s.Kind != "function" && s.Kind != "method" {
			continue
		}
		if line < s.LineStart || line > s.LineEnd {
			continue
		}
		if best == nil || (s.LineEnd-s.LineStart) < (best.LineEnd-best.LineStart) {
			best = s
		}
	}
	return best
}

func replaceEdges(ctx context.Context, db *sql.DB, byFile map[string][]Symbol, edges []Edge) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("codegraph: begin tx: %w", err)
	}
	defer tx.Rollback()

	for file := range byFile {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM edges WHERE from_symbol IN (SELECT id FROM symbols WHERE file = ?)`, file); err != nil {
			return fmt.Errorf("codegraph: delete old edges for %s: %w", file, err)
		}
	}
	for _, e := range edges {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO edges (from_symbol, to_symbol, kind, file, line) VALUES (?, ?, ?, ?, ?)`,
			e.FromSymbol, e.ToSymbol, e.Kind, e.File, e.Line,
		); err != nil {
			return fmt.Errorf("codegraph: insert edge %s->%s: %w", e.FromSymbol, e.ToSymbol, err)
		}
	}
	return tx.Commit()
}

// walkSourceFiles junta los .go del árbol (respetando ignore) y cuenta
// por extensión todo lo demás que parezca código fuente -- para poder
// reportar qué quedó sin indexar y por qué (requisito 9).
func walkSourceFiles(rootDir string, ignore []string) (goFiles []string, otherExts map[string]int, err error) {
	otherExts = map[string]int{}
	knownSourceExts := map[string]bool{".py": true, ".ts": true, ".tsx": true, ".js": true, ".rs": true, ".java": true}

	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(rootDir, path)
		if info.IsDir() {
			if isIgnored(rel+"/", ignore) {
				return filepath.SkipDir
			}
			return nil
		}
		if isIgnored(rel, ignore) {
			return nil
		}
		ext := filepath.Ext(path)
		switch {
		case ext == ".go":
			goFiles = append(goFiles, path)
		case knownSourceExts[ext]:
			otherExts[ext]++
		}
		return nil
	})
	return goFiles, otherExts, err
}

func isIgnored(relPath string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasPrefix(p, "*") {
			if strings.HasSuffix(relPath, strings.TrimPrefix(p, "*")) {
				return true
			}
			continue
		}
		if strings.HasPrefix(relPath, p) {
			return true
		}
	}
	return false
}
