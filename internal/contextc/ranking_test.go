package contextc

import "testing"

func TestRank_SortsByRelevanceDescending(t *testing.T) {
	in := []Fragment{{Ref: "low", Relevance: 0.2}, {Ref: "high", Relevance: 0.9}, {Ref: "mid", Relevance: 0.5}}
	out := Rank(in)
	if out[0].Ref != "high" || out[1].Ref != "mid" || out[2].Ref != "low" {
		t.Fatalf("orden incorrecto: %+v", out)
	}
	// Rank no debe mutar el slice de entrada.
	if in[0].Ref != "low" {
		t.Fatal("Rank mutó el slice de entrada")
	}
}
