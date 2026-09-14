package contextc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFiles_FindsPathsAndFilenames(t *testing.T) {
	got := extractFiles("refactor el TaskProfile en internal/router y revisá config.yaml")
	want := map[string]bool{"internal/router": true, "config.yaml": true}
	if len(got) != 2 {
		t.Fatalf("esperaba 2 archivos, got %+v", got)
	}
	for _, f := range got {
		if !want[f] {
			t.Fatalf("archivo inesperado %q en %+v", f, got)
		}
	}
}

func TestExtractSymbols_FindsPascalCaseIdentifiers(t *testing.T) {
	got := extractSymbols("refactor el TaskProfile y el RoutingDecision")
	if len(got) != 2 || got[0] != "TaskProfile" || got[1] != "RoutingDecision" {
		t.Fatalf("esperaba [TaskProfile RoutingDecision], got %+v", got)
	}
}

func TestCapByBudget_StopsAtFirstThatDoesNotFit(t *testing.T) {
	frags := []Fragment{{Ref: "a", Tokens: 50}, {Ref: "b", Tokens: 60}, {Ref: "c", Tokens: 5}}
	got := capByBudget(frags, 100)
	if len(got) != 1 || got[0].Ref != "a" {
		t.Fatalf("esperaba solo 'a' (corta en el primero que no entra, no bin-packing), got %+v", got)
	}
}

func TestReadLines_ReturnsBoundedRange(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readLines(dir, "f.go", 2, 4)
	if err != nil {
		t.Fatalf("readLines: %v", err)
	}
	if got != "line2\nline3\nline4" {
		t.Fatalf("got %q", got)
	}
}

func TestFileSource_SkipsPathsWithoutExtension(t *testing.T) {
	dir := t.TempDir()
	q := Query{Project: dir, Files: []string{"internal/router", "missing.go"}}
	frags, err := (&FileSource{}).Retrieve(context.Background(), q, 1000)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(frags) != 0 {
		t.Fatalf("esperaba 0 (sin extensión se salta, 'missing.go' no existe), got %+v", frags)
	}
}

func TestFileSource_ReadsFileWithExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0o644); err != nil {
		t.Fatal(err)
	}
	q := Query{Project: dir, Files: []string{"a.go"}}
	frags, err := (&FileSource{}).Retrieve(context.Background(), q, 1000)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(frags) != 1 || frags[0].Content != "package a" {
		t.Fatalf("got %+v", frags)
	}
}

func TestSkillsSource_AlwaysEmpty(t *testing.T) {
	frags, err := (&SkillsSource{}).Retrieve(context.Background(), Query{}, 1000)
	if err != nil || frags != nil {
		t.Fatalf("SkillsSource debe ser siempre vacío (hook de Fase 5), got %+v, %v", frags, err)
	}
}
