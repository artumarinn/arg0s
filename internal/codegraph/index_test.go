package codegraph

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsIgnored(t *testing.T) {
	patterns := []string{"vendor/", "node_modules/", "*.min.js"}
	cases := []struct {
		path string
		want bool
	}{
		{"vendor/pkg/file.go", true},
		{"node_modules/x/index.js", true},
		{"app.min.js", true},
		{"internal/router/router.go", false},
		{"main.go", false},
	}
	for _, c := range cases {
		if got := isIgnored(c.path, patterns); got != c.want {
			t.Errorf("isIgnored(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestWalkSourceFiles_FindsGoAndCountsOtherLanguages(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "vendor", "pkg"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "vendor", "pkg", "ignored.go"), []byte("package pkg"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "script.py"), []byte("print(1)"), 0o644))

	goFiles, otherExts, err := walkSourceFiles(dir, []string{"vendor/"})
	if err != nil {
		t.Fatalf("walkSourceFiles: %v", err)
	}
	if len(goFiles) != 1 || filepath.Base(goFiles[0]) != "main.go" {
		t.Fatalf("esperaba solo main.go, got %+v", goFiles)
	}
	if otherExts[".py"] != 1 {
		t.Fatalf("esperaba 1 archivo .py contado, got %+v", otherExts)
	}
}

func TestSymbolAtLine_PicksInnermostFunction(t *testing.T) {
	symbols := []Symbol{
		{ID: "outer", Kind: "function", LineStart: 1, LineEnd: 20},
		{ID: "inner", Kind: "method", LineStart: 5, LineEnd: 10},
		{ID: "type", Kind: "type", LineStart: 1, LineEnd: 20}, // no es function/method, se ignora
	}
	got := symbolAtLine(symbols, 7)
	if got == nil || got.ID != "inner" {
		t.Fatalf("esperaba 'inner' (rango más específico), got %+v", got)
	}
}

func TestSymbolAtLine_NoMatch_ReturnsNil(t *testing.T) {
	symbols := []Symbol{{ID: "f", Kind: "function", LineStart: 1, LineEnd: 5}}
	if got := symbolAtLine(symbols, 100); got != nil {
		t.Fatalf("esperaba nil, got %+v", got)
	}
}

func TestFlattenSymbols_MapsKindsAndCapturesSelectionPosition(t *testing.T) {
	docSymbols := []documentSymbol{
		{
			Name: "Executor", Kind: 5, // Class -> type
			Range:          lspRange{Start: lspPosition{Line: 9}, End: lspPosition{Line: 30}},
			SelectionRange: lspRange{Start: lspPosition{Line: 9, Character: 5}},
			Children: []documentSymbol{
				{
					Name: "Execute", Kind: 6, // Method
					Range:          lspRange{Start: lspPosition{Line: 12}, End: lspPosition{Line: 20}},
					SelectionRange: lspRange{Start: lspPosition{Line: 12, Character: 20}},
				},
			},
		},
	}
	symbols, positions := flattenSymbols(docSymbols, "arg0s", "executor.go", time.Now())

	if len(symbols) != 2 {
		t.Fatalf("esperaba 2 symbols (type + method), got %d: %+v", len(symbols), symbols)
	}
	if symbols[0].Kind != "type" || symbols[1].Kind != "method" {
		t.Fatalf("kinds mapeados mal: %+v", symbols)
	}
	if symbols[0].LineStart != 10 { // LSP es 0-indexed, symbols es 1-indexed
		t.Fatalf("LineStart = %d, want 10", symbols[0].LineStart)
	}
	pos, ok := positions[symbols[1].ID]
	if !ok || pos.Character != 20 {
		t.Fatalf("esperaba selectionRange capturada para el method, got %+v ok=%v", pos, ok)
	}
}

func TestRelFromURI(t *testing.T) {
	root := "/home/user/repo"
	rel, ok := relFromURI(root, "file:///home/user/repo/internal/router/router.go")
	if !ok || rel != "internal/router/router.go" {
		t.Fatalf("relFromURI = %q, %v", rel, ok)
	}
	if _, ok := relFromURI(root, "file:///etc/passwd"); ok {
		t.Fatal("esperaba ok=false para un path fuera de root")
	}
}
