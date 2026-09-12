package router

import (
	"testing"

	"github.com/artumarinn/arg0s/internal/core"
)

func TestClassifyComplexity(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   core.Level
	}{
		{"corto sin código", "cuánto es 2+2", core.LevelLow},
		{"pide código, longitud media", "implementa una función que sume dos números en internal/util", core.LevelMedium},
		{"menciona arquitectura", "diseñá la arquitectura de X", core.LevelHigh},
		{"menciona refactor", "refactoriza este módulo para separar responsabilidades", core.LevelHigh},
		{"muy largo", string(make([]byte, 1001)), core.LevelHigh},
		{"más de 3 archivos", "revisá auth.go, token.go, session.go y router.go", core.LevelHigh},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyComplexity(c.prompt); got != c.want {
				t.Fatalf("classifyComplexity(%q) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

func TestClassifyType(t *testing.T) {
	cases := []struct {
		prompt string
		want   core.TaskType
	}{
		{"refactoriza este módulo", core.TaskTypeRefactor},
		{"revisa este PR", core.TaskTypeReview},
		{"audita este código", core.TaskTypeReview},
		{"diseñá la arquitectura de X", core.TaskTypeArchitecture},
		{"implementa un parser", core.TaskTypeCoding},
		{"escribe un test", core.TaskTypeCoding},
		{"qué hace esta función", core.TaskTypeQA},
		{"cómo se usa esto", core.TaskTypeQA},
		{"cuánto es 2+2", core.TaskTypeQA},
		{"sin ningún keyword conocido aquí", core.TaskTypeQA},
	}
	for _, c := range cases {
		t.Run(c.prompt, func(t *testing.T) {
			if got := classifyType(c.prompt); got != c.want {
				t.Fatalf("classifyType(%q) = %q, want %q", c.prompt, got, c.want)
			}
		})
	}
}

func TestClassifyPrivacy_NoGitRepo_IsPublic(t *testing.T) {
	if got := classifyPrivacy(t.TempDir()); got != core.PrivacyPublic {
		t.Fatalf("classifyPrivacy(sin repo) = %q, want public", got)
	}
}
