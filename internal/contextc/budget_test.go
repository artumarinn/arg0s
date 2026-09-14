package contextc

import (
	"context"
	"strings"
	"testing"
)

type fakeSummarizer struct {
	compressTo int // devuelve un texto de este largo, simulando compresión real
	calls      int
}

func (f *fakeSummarizer) Compress(ctx context.Context, content string, targetTokens int) (string, error) {
	f.calls++
	if f.compressTo >= len(content) {
		return content, nil // "no logró comprimir" -- mismo largo o más
	}
	return strings.Repeat("x", f.compressTo), nil
}

func TestApplyBudget_UnderBudget_IncludesEverything(t *testing.T) {
	frags := []Fragment{{Ref: "a", Tokens: 10}, {Ref: "b", Tokens: 10}}
	included, excluded := applyBudget(context.Background(), "code", 100, frags, nil)
	if len(included) != 2 || len(excluded) != 0 {
		t.Fatalf("esperaba todo incluido, got included=%d excluded=%d", len(included), len(excluded))
	}
}

func TestApplyBudget_NoSummarizer_ExcludesLeastRelevantFirst(t *testing.T) {
	// candidates viene ordenado por relevancia desc (a > b) -- b debe
	// ser el excluido, no a.
	frags := []Fragment{{Ref: "a", Tokens: 60}, {Ref: "b", Tokens: 60}}
	included, excluded := applyBudget(context.Background(), "code", 100, frags, nil)
	if len(included) != 1 || included[0].Ref != "a" {
		t.Fatalf("esperaba solo 'a' incluido, got %+v", included)
	}
	if len(excluded) != 1 || excluded[0].Ref != "b" {
		t.Fatalf("esperaba 'b' excluido con razón, got %+v", excluded)
	}
	if excluded[0].Reason == "" {
		t.Fatal("excluded debe traer Reason")
	}
}

func TestApplyBudget_WithSummarizer_CompressesLargestFragmentFirst(t *testing.T) {
	// "a" es más grande que "b" -- debe comprimirse "a", no descartar
	// "b" directo (sección 9 punto 12).
	frags := []Fragment{{Ref: "a", Tokens: 90, Content: strings.Repeat("A", 360)}, {Ref: "b", Tokens: 10, Content: strings.Repeat("B", 40)}}
	summarizer := &fakeSummarizer{compressTo: 40} // 40 chars ~= 10 tokens
	included, excluded := applyBudget(context.Background(), "code", 20, frags, summarizer)

	if len(excluded) != 0 {
		t.Fatalf("esperaba 0 excluidos (la compresión alcanzó), got %+v", excluded)
	}
	if summarizer.calls == 0 {
		t.Fatal("esperaba que se llame al summarizer")
	}
	var gotA bool
	for _, f := range included {
		if f.Ref == "a (comprimido)" {
			gotA = true
		}
	}
	if !gotA {
		t.Fatalf("esperaba 'a' marcado como comprimido, got %+v", included)
	}
}

func TestApplyBudget_SummarizerCantHelp_FallsBackToExclusion(t *testing.T) {
	frags := []Fragment{{Ref: "a", Tokens: 60, Content: strings.Repeat("A", 240)}, {Ref: "b", Tokens: 60, Content: strings.Repeat("B", 240)}}
	summarizer := &fakeSummarizer{compressTo: 1000} // "comprime" a algo más grande -- no ayuda
	included, excluded := applyBudget(context.Background(), "code", 100, frags, summarizer)

	if len(excluded) != 1 {
		t.Fatalf("esperaba fallback a exclusión, got included=%+v excluded=%+v", included, excluded)
	}
}
