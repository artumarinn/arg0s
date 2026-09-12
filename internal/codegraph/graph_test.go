package codegraph

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/artumarinn/arg0s/internal/storage"
)

// openTestGraph usa storage.Open real (mismo criterio que
// internal/memory/sqlite: symbols/edges es una tabla local, no hay
// excusa para mockear el store).
func openTestGraph(t *testing.T) *Graph {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "arg0s.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewGraph(db.DB)
}

func insertSymbol(t *testing.T, g *Graph, s Symbol) {
	t.Helper()
	if s.IndexedAt.IsZero() {
		s.IndexedAt = time.Now()
	}
	_, err := g.db.Exec(`
		INSERT INTO symbols (id, project, name, kind, file, line_start, line_end, signature, doc, language, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Project, s.Name, s.Kind, s.File, s.LineStart, s.LineEnd, s.Signature, s.Doc, s.Language, s.IndexedAt.Unix())
	if err != nil {
		t.Fatalf("insert symbol: %v", err)
	}
}

func insertEdge(t *testing.T, g *Graph, e Edge) {
	t.Helper()
	_, err := g.db.Exec(`INSERT INTO edges (from_symbol, to_symbol, kind, file, line) VALUES (?, ?, ?, ?, ?)`,
		e.FromSymbol, e.ToSymbol, e.Kind, e.File, e.Line)
	if err != nil {
		t.Fatalf("insert edge: %v", err)
	}
}

func TestFindSymbol_ReturnsMatchesForProject(t *testing.T) {
	g := openTestGraph(t)
	insertSymbol(t, g, Symbol{ID: "p::a.go::10::TaskProfile", Project: "arg0s", Name: "TaskProfile", Kind: "type", File: "a.go", LineStart: 10, LineEnd: 20})
	insertSymbol(t, g, Symbol{ID: "p2::a.go::10::TaskProfile", Project: "otro", Name: "TaskProfile", Kind: "type", File: "a.go", LineStart: 10, LineEnd: 20})

	got, err := g.FindSymbol(context.Background(), "arg0s", "TaskProfile")
	if err != nil {
		t.Fatalf("FindSymbol: %v", err)
	}
	if len(got) != 1 || got[0].Project != "arg0s" {
		t.Fatalf("esperaba 1 resultado del proyecto arg0s, got %+v", got)
	}
}

func TestDefinition_NotFound_ReturnsErrSymbolNotFound(t *testing.T) {
	g := openTestGraph(t)
	_, err := g.Definition(context.Background(), "arg0s", "NoExiste")
	if err != ErrSymbolNotFound {
		t.Fatalf("expected ErrSymbolNotFound, got %v", err)
	}
}

func TestCallers_ReturnsFunctionsThatCallTarget(t *testing.T) {
	g := openTestGraph(t)
	insertSymbol(t, g, Symbol{ID: "target", Project: "arg0s", Name: "Greet", Kind: "function", File: "greet.go", LineStart: 1, LineEnd: 3})
	insertSymbol(t, g, Symbol{ID: "caller", Project: "arg0s", Name: "main", Kind: "function", File: "main.go", LineStart: 1, LineEnd: 5})
	insertEdge(t, g, Edge{FromSymbol: "caller", ToSymbol: "target", Kind: "calls", File: "main.go", Line: 3})

	callers, err := g.Callers(context.Background(), "arg0s", "target", 1)
	if err != nil {
		t.Fatalf("Callers: %v", err)
	}
	if len(callers) != 1 || callers[0].ID != "caller" {
		t.Fatalf("esperaba [caller], got %+v", callers)
	}
}

func TestCallees_ReturnsWhatTargetCalls(t *testing.T) {
	g := openTestGraph(t)
	insertSymbol(t, g, Symbol{ID: "caller", Project: "arg0s", Name: "main", Kind: "function", File: "main.go", LineStart: 1, LineEnd: 5})
	insertSymbol(t, g, Symbol{ID: "target", Project: "arg0s", Name: "Greet", Kind: "function", File: "greet.go", LineStart: 1, LineEnd: 3})
	insertEdge(t, g, Edge{FromSymbol: "caller", ToSymbol: "target", Kind: "calls", File: "main.go", Line: 3})

	callees, err := g.Callees(context.Background(), "arg0s", "caller", 1)
	if err != nil {
		t.Fatalf("Callees: %v", err)
	}
	if len(callees) != 1 || callees[0].ID != "target" {
		t.Fatalf("esperaba [target], got %+v", callees)
	}
}

func TestImpactSet_IncludesSymbolFileAndCallerFiles(t *testing.T) {
	g := openTestGraph(t)
	insertSymbol(t, g, Symbol{ID: "target", Project: "arg0s", Name: "Greet", Kind: "function", File: "greet.go", LineStart: 1, LineEnd: 3})
	insertSymbol(t, g, Symbol{ID: "caller", Project: "arg0s", Name: "main", Kind: "function", File: "main.go", LineStart: 1, LineEnd: 5})
	insertEdge(t, g, Edge{FromSymbol: "caller", ToSymbol: "target", Kind: "calls", File: "main.go", Line: 3})

	files, err := g.ImpactSet(context.Background(), "arg0s", []string{"target"})
	if err != nil {
		t.Fatalf("ImpactSet: %v", err)
	}
	paths := map[string]bool{}
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["greet.go"] || !paths["main.go"] {
		t.Fatalf("esperaba greet.go y main.go, got %+v", files)
	}
}

func TestRelatedFiles_MatchesSubstringOfName(t *testing.T) {
	g := openTestGraph(t)
	insertSymbol(t, g, Symbol{ID: "a", Project: "arg0s", Name: "TaskProfile", Kind: "type", File: "task.go", LineStart: 1, LineEnd: 5})
	insertSymbol(t, g, Symbol{ID: "b", Project: "arg0s", Name: "ProfileHeuristic", Kind: "function", File: "heuristics.go", LineStart: 1, LineEnd: 5})
	insertSymbol(t, g, Symbol{ID: "c", Project: "arg0s", Name: "Unrelated", Kind: "function", File: "other.go", LineStart: 1, LineEnd: 5})

	files, err := g.RelatedFiles(context.Background(), "arg0s", "Profile", 10)
	if err != nil {
		t.Fatalf("RelatedFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("esperaba 2 archivos, got %+v", files)
	}
}
