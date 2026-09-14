package codegraph

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Graph son las queries de sección 13 expuestas al Context Compiler
// (y por ahora, a `arg0s graph`). Opera sobre el mismo *sql.DB de
// arg0s.db -- no importa internal/storage (mismo patrón que
// internal/memory/sqlite).
type Graph struct {
	db *sql.DB
}

func NewGraph(db *sql.DB) *Graph { return &Graph{db: db} }

func (g *Graph) FindSymbol(ctx context.Context, project, name string) ([]Symbol, error) {
	rows, err := g.db.QueryContext(ctx, `
		SELECT id, project, name, kind, file, line_start, line_end, COALESCE(signature,''), COALESCE(doc,''), COALESCE(language,''), indexed_at
		FROM symbols WHERE project = ? AND name = ? ORDER BY file, line_start`, project, name)
	if err != nil {
		return nil, fmt.Errorf("codegraph: find symbol %q: %w", name, err)
	}
	defer rows.Close()
	return scanSymbols(rows)
}

var ErrSymbolNotFound = fmt.Errorf("codegraph: symbol no encontrado")

// Definition devuelve el primer símbolo que matchea ref -- por id
// exacto si ref tiene esa forma, si no por nombre exacto.
func (g *Graph) Definition(ctx context.Context, project, ref string) (Symbol, error) {
	row := g.db.QueryRowContext(ctx, `
		SELECT id, project, name, kind, file, line_start, line_end, COALESCE(signature,''), COALESCE(doc,''), COALESCE(language,''), indexed_at
		FROM symbols WHERE project = ? AND (id = ? OR name = ?) ORDER BY (id = ?) DESC LIMIT 1`, project, ref, ref, ref)
	return scanSymbol(row)
}

// Callers devuelve quién llama a symbol -- edges donde symbol es el
// to_symbol, kind=calls. depth<=1 en Fase 3 (sin transitividad
// todavía; se puede pedir cuando el Context Compiler la necesite).
func (g *Graph) Callers(ctx context.Context, project, symbolID string, depth int) ([]Symbol, error) {
	return g.related(ctx, project, `
		SELECT s.id, s.project, s.name, s.kind, s.file, s.line_start, s.line_end, COALESCE(s.signature,''), COALESCE(s.doc,''), COALESCE(s.language,''), s.indexed_at
		FROM symbols s JOIN edges e ON e.from_symbol = s.id
		WHERE e.to_symbol = ? AND e.kind = 'calls' AND s.project = ?`, symbolID)
}

// Callees devuelve a quién llama symbol -- edges donde symbol es el
// from_symbol.
func (g *Graph) Callees(ctx context.Context, project, symbolID string, depth int) ([]Symbol, error) {
	return g.related(ctx, project, `
		SELECT s.id, s.project, s.name, s.kind, s.file, s.line_start, s.line_end, COALESCE(s.signature,''), COALESCE(s.doc,''), COALESCE(s.language,''), s.indexed_at
		FROM symbols s JOIN edges e ON e.to_symbol = s.id
		WHERE e.from_symbol = ? AND e.kind = 'calls' AND s.project = ?`, symbolID)
}

func (g *Graph) related(ctx context.Context, project, query, symbolID string) ([]Symbol, error) {
	rows, err := g.db.QueryContext(ctx, query, symbolID, project)
	if err != nil {
		return nil, fmt.Errorf("codegraph: related query: %w", err)
	}
	defer rows.Close()
	return scanSymbols(rows)
}

// ImpactSet devuelve los archivos que un cambio en symbols toca --
// el archivo de cada symbol más el de cada caller directo (sección
// 13: "qué archivos toca un cambio").
func (g *Graph) ImpactSet(ctx context.Context, project string, symbolIDs []string) ([]File, error) {
	seen := map[string]bool{}
	var out []File
	for _, id := range symbolIDs {
		def, err := g.Definition(ctx, project, id)
		if err == nil && !seen[def.File] {
			seen[def.File] = true
			out = append(out, File{Path: def.File})
		}
		callers, err := g.Callers(ctx, project, id, 1)
		if err != nil {
			return nil, err
		}
		for _, c := range callers {
			if !seen[c.File] {
				seen[c.File] = true
				out = append(out, File{Path: c.File})
			}
		}
	}
	return out, nil
}

// SymbolsInFile lista los symbols indexados de un archivo -- usado por
// el Context Compiler para leer TODO lo relevante de un archivo que un
// hint de path apuntó, sin tener ya un nombre de símbolo exacto.
func (g *Graph) SymbolsInFile(ctx context.Context, project, file string) ([]Symbol, error) {
	rows, err := g.db.QueryContext(ctx, `
		SELECT id, project, name, kind, file, line_start, line_end, COALESCE(signature,''), COALESCE(doc,''), COALESCE(language,''), indexed_at
		FROM symbols WHERE project = ? AND file = ? ORDER BY line_start`, project, file)
	if err != nil {
		return nil, fmt.Errorf("codegraph: symbols in file %q: %w", file, err)
	}
	defer rows.Close()
	return scanSymbols(rows)
}

// RelatedFiles busca símbolos cuyo nombre matchea query (substring,
// sin ranking semántico -- mismo espíritu lexical que
// internal/memory/sqlite.Relevance) y devuelve los archivos donde
// viven, sin repetir.
func (g *Graph) RelatedFiles(ctx context.Context, project, query string, limit int) ([]File, error) {
	rows, err := g.db.QueryContext(ctx, `
		SELECT DISTINCT file FROM symbols WHERE project = ? AND name LIKE ? ORDER BY file LIMIT ?`,
		project, "%"+query+"%", limitOrDefault(limit))
	if err != nil {
		return nil, fmt.Errorf("codegraph: related files: %w", err)
	}
	defer rows.Close()

	var out []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.Path); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func limitOrDefault(limit int) int {
	if limit <= 0 {
		return 50
	}
	return limit
}

func scanSymbols(rows *sql.Rows) ([]Symbol, error) {
	var out []Symbol
	for rows.Next() {
		s, err := scanSymbolRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSymbol(row scanner) (Symbol, error) {
	s, err := scanSymbolRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Symbol{}, ErrSymbolNotFound
		}
		return Symbol{}, err
	}
	return s, nil
}

func scanSymbolRow(row scanner) (Symbol, error) {
	var s Symbol
	var indexedAt int64
	err := row.Scan(&s.ID, &s.Project, &s.Name, &s.Kind, &s.File, &s.LineStart, &s.LineEnd, &s.Signature, &s.Doc, &s.Language, &indexedAt)
	if err != nil {
		return Symbol{}, err
	}
	s.IndexedAt = time.Unix(indexedAt, 0)
	return s, nil
}
