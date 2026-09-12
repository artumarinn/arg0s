//go:build integration

// Este archivo corre SOLO con `go test -tags=integration` -- necesita
// gopls real instalado (regla dura #8: go test ./... nunca toca la
// red ni depende de binarios externos). Arma un módulo Go fixture de
// dos archivos y verifica que Index() encuentra symbols reales y la
// edge "calls" via textDocument/references, no un mock.
package codegraph

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/artumarinn/arg0s/internal/storage"
)

func TestIndex_AgainstRealGopls_FindsSymbolsAndCallEdge(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls no está en el PATH -- instalar con: go install golang.org/x/tools/gopls@latest")
	}

	dir := t.TempDir()
	writeFixtureModule(t, dir)

	db, err := storage.Open(filepath.Join(t.TempDir(), "arg0s.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stats, err := Index(ctx, "gopls", []string{"serve"}, dir, dir, db.DB, nil)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if stats.FilesIndexed != 2 {
		t.Fatalf("FilesIndexed = %d, want 2", stats.FilesIndexed)
	}
	if stats.SymbolsIndexed == 0 {
		t.Fatal("esperaba al menos 1 symbol indexado")
	}

	graph := NewGraph(db.DB)
	greet, err := graph.FindSymbol(ctx, dir, "Greet")
	if err != nil || len(greet) != 1 {
		t.Fatalf("FindSymbol(Greet) = %+v, %v", greet, err)
	}

	callers, err := graph.Callers(ctx, dir, greet[0].ID, 1)
	if err != nil {
		t.Fatalf("Callers: %v", err)
	}
	if len(callers) != 1 || callers[0].Name != "main" {
		t.Fatalf("esperaba que main() aparezca como caller de Greet, got %+v", callers)
	}
}

func writeFixtureModule(t *testing.T, dir string) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.21\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "greet.go"), []byte(`package main

func Greet(name string) string {
	return "hola " + name
}
`), 0o644))
	must(os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import "fmt"

func main() {
	fmt.Println(Greet("arg0s"))
}
`), 0o644))
}
