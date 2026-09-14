package contextc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/artumarinn/arg0s/internal/codegraph"
	"github.com/artumarinn/arg0s/internal/config"
	memcore "github.com/artumarinn/arg0s/internal/memory"
	memsqlite "github.com/artumarinn/arg0s/internal/memory/sqlite"
	"github.com/artumarinn/arg0s/internal/storage"
)

func testCompilerSetup(t *testing.T) (project string, memStore memcore.MemoryStore, graph *codegraph.Graph) {
	t.Helper()
	project = t.TempDir()
	db, err := storage.Open(filepath.Join(t.TempDir(), "arg0s.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Fixture de código real -- el Context Compiler lee contenido de
	// disco acotado al rango del symbol, no solo metadata.
	src := "package core\n\ntype TaskProfile struct {\n\tType string\n}\n"
	if err := os.WriteFile(filepath.Join(project, "task.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`
		INSERT INTO symbols (id, project, name, kind, file, line_start, line_end, signature, doc, language, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"sym1", project, "TaskProfile", "type", "task.go", 3, 5, "", "", "go", time.Now().Unix()); err != nil {
		t.Fatalf("insert symbol: %v", err)
	}

	memStore = memsqlite.New(db.DB, 0)
	if err := memStore.Store(context.Background(), memcore.Memory{
		Project: project, Category: memcore.CategoryArchitecture,
		Title: "TaskProfile", Content: "TaskProfile vive en task.go y lo llena el router",
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	graph = codegraph.NewGraph(db.DB)
	return project, memStore, graph
}

func TestCompile_IncludesPromptMemoryAndCode_RespectsManifest(t *testing.T) {
	project, memStore, graph := testCompilerSetup(t)
	cfg := config.ContextConfig{
		DefaultBudgetTokens: 16000,
		BudgetSplit:         map[string]float64{"prompt": 0.10, "code": 0.45, "graph": 0.10, "memory": 0.20, "skills": 0.10, "artifacts": 0.05},
	}
	compiler := New(cfg, memStore, 0.1, graph, nil)

	cc, err := compiler.Compile(context.Background(), "refactor el TaskProfile en internal/router", project)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	if cc.Budget != 16000 {
		t.Fatalf("Budget = %d, want 16000", cc.Budget)
	}
	if cc.TotalTokens <= 0 || cc.TotalTokens > cc.Budget {
		t.Fatalf("TotalTokens = %d fuera de rango (budget=%d)", cc.TotalTokens, cc.Budget)
	}

	var sources map[string]bool = map[string]bool{}
	for _, f := range cc.Fragments {
		sources[f.Source] = true
	}
	if !sources["prompt"] {
		t.Fatal("esperaba el fragmento 'prompt' siempre incluido")
	}
	if !sources["memory"] {
		t.Fatalf("esperaba un fragmento de memory, fragments=%+v", cc.Fragments)
	}
	if !sources["codegraph"] {
		t.Fatalf("esperaba un fragmento de codegraph (TaskProfile localizado), fragments=%+v", cc.Fragments)
	}

	if len(cc.Manifest.Included) != len(cc.Fragments) {
		t.Fatalf("Manifest.Included (%d) no matchea Fragments (%d)", len(cc.Manifest.Included), len(cc.Fragments))
	}
}

func TestCompile_TinyBudget_ExcludesWithReasonsInManifest(t *testing.T) {
	project, memStore, graph := testCompilerSetup(t)
	cfg := config.ContextConfig{
		DefaultBudgetTokens: 10, // fuerza exclusión -- casi nada entra
		BudgetSplit:         map[string]float64{"prompt": 0.10, "code": 0.45, "graph": 0.10, "memory": 0.20, "skills": 0.10, "artifacts": 0.05},
	}
	compiler := New(cfg, memStore, 0.0, graph, nil) // sin summarizer -- fuerza el fallback de exclusión

	cc, err := compiler.Compile(context.Background(), "refactor el TaskProfile en internal/router", project)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(cc.Manifest.Excluded) == 0 {
		t.Fatal("esperaba al menos 1 excluido con budget=10")
	}
	for _, ex := range cc.Manifest.Excluded {
		if ex.Reason == "" {
			t.Fatalf("excluido sin razón: %+v", ex)
		}
	}
}
