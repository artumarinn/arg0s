package fusion

import "testing"

func TestMeasure_NoSecondaries_ZeroDivergence(t *testing.T) {
	d := Measure("lexical", "cualquier cosa", nil)
	if d.Score != 0 {
		t.Fatalf("Score = %v, want 0", d.Score)
	}
}

func TestMeasure_IdenticalText_ZeroDivergence(t *testing.T) {
	d := Measure("lexical", "la respuesta es 42", []string{"la respuesta es 42"})
	if d.Score != 0 {
		t.Fatalf("Score = %v, want 0", d.Score)
	}
}

func TestMeasure_CompletelyDifferentText_HighDivergence(t *testing.T) {
	d := Measure("lexical", "gatos perros parque jugando", []string{"xilofono cuantico marte volcan"})
	if d.Score < 0.9 {
		t.Fatalf("Score = %v, want cerca de 1 (sin overlap)", d.Score)
	}
}

func TestMeasure_UnknownMethod_DegradesToLexical(t *testing.T) {
	d := Measure("judge", "hola mundo", []string{"hola mundo"})
	if d.Method != "lexical" || d.Score != 0 {
		t.Fatalf("esperaba degradar a lexical, got %+v", d)
	}
}

func TestMeasure_WorstCaseAmongSecondaries(t *testing.T) {
	// un secundario coincide, otro no -- debe ganar el peor caso (mayor
	// divergencia), no un promedio que lo esconda.
	primary := "la respuesta correcta es sumar los dos numeros"
	agree := "la respuesta correcta es sumar los dos numeros"
	disagree := "totalmente distinto sin relacion alguna aca marte volcan"
	d := Measure("lexical", primary, []string{agree, disagree})
	if d.Score < 0.5 {
		t.Fatalf("Score = %v, esperaba que domine el secundario que discrepa", d.Score)
	}
}

func TestLexicalDivergence_CodeWithRenamedVariables_LowDivergence(t *testing.T) {
	a := "func Sum() int { x := 1; y := 2; return x + y }"
	b := "func Sum() int { a := 1; b := 2; return a + b }"
	d := lexicalDivergence(a, b)
	if d > 0.15 {
		t.Fatalf("divergencia = %.2f, esperaba baja (mismas variables renombradas no son divergencia real, sección 10.2)", d)
	}
}

func TestLexicalDivergence_CodeWithDifferentControlFlow_HigherDivergence(t *testing.T) {
	same := "func Sum() int { x := 1; y := 2; return x + y }"
	different := "func Sum() int { x := 1; if x > 0 { return x } return 0 }"
	d := lexicalDivergence(same, different)
	if d < 0.2 {
		t.Fatalf("divergencia = %.2f, esperaba que un if de más se note (no es solo renombre de variables)", d)
	}
}
