package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/artumarinn/arg0s/internal/contextc"
)

func TestCodeVsBaselineTokens_ComparesFullFileToBoundedFragment(t *testing.T) {
	dir := t.TempDir()
	full := "package a\n\nfunc A() {}\nfunc B() {}\nfunc C() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}

	cc := contextc.CompiledContext{
		Fragments: []contextc.Fragment{
			{Source: "prompt", Ref: "prompt", Tokens: 5},
			{Source: "codegraph", Ref: "a.go:3-3", Content: "func A() {}", Tokens: contextc.EstimateTokens("func A() {}")},
		},
	}

	compiled, baseline, files := codeVsBaselineTokens(dir, cc)
	if len(files) != 1 || files[0] != "a.go" {
		t.Fatalf("files = %+v, want [a.go]", files)
	}
	if compiled != contextc.EstimateTokens("func A() {}") {
		t.Fatalf("compiled = %d", compiled)
	}
	wantBaseline := contextc.EstimateTokens(full)
	if baseline != wantBaseline {
		t.Fatalf("baseline = %d, want %d", baseline, wantBaseline)
	}
	if baseline <= compiled {
		t.Fatalf("esperaba baseline (%d) > compiled (%d) -- el bounded fragment debe ser más chico que el archivo completo", baseline, compiled)
	}
}

func TestCodeVsBaselineTokens_IgnoresMemoryAndPromptFragments(t *testing.T) {
	cc := contextc.CompiledContext{
		Fragments: []contextc.Fragment{
			{Source: "prompt", Ref: "prompt", Tokens: 100},
			{Source: "memory", Ref: "alguna memoria", Tokens: 100},
		},
	}
	compiled, baseline, files := codeVsBaselineTokens(t.TempDir(), cc)
	if compiled != 0 || baseline != 0 || len(files) != 0 {
		t.Fatalf("esperaba todo en 0 (sin fragmentos de código), got compiled=%d baseline=%d files=%+v", compiled, baseline, files)
	}
}
