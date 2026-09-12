package main

import (
	"bytes"
	"testing"

	"github.com/artumarinn/arg0s/internal/core"
)

func TestPrintFallbackWarning_WhenFellBack_PrintsWarning(t *testing.T) {
	var buf bytes.Buffer
	printFallbackWarning(&buf, core.ModelRun{ModelID: "qwen-coder-14b", FellBackFrom: "gemini-flash"})

	want := "⚠ gemini-flash no disponible, usando fallback: qwen-coder-14b\n"
	if got := buf.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrintFallbackWarning_WhenNoFallback_PrintsNothing(t *testing.T) {
	var buf bytes.Buffer
	printFallbackWarning(&buf, core.ModelRun{ModelID: "gemini-flash"})

	if buf.Len() != 0 {
		t.Fatalf("expected no output, got %q", buf.String())
	}
}
