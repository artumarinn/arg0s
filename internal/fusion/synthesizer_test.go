package fusion

import (
	"context"
	"testing"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/execution"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/mock"
)

func TestExecutorSynthesizer_ParsesWinnerAndReturnsItsContent(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(mock.New(mock.Config{
		Name:     "synthmodel",
		Response: `{"winner": 2, "reasoning": "la segunda maneja mejor el error", "borrowed_blocks": []}`,
	}))
	models := &config.ModelsFile{Models: map[string]config.ModelConfig{"synthmodel": {Provider: "synthmodel"}}}
	exec := execution.New(reg, models, map[string]execution.ProviderPolicy{}, nil)

	s := &ExecutorSynthesizer{Exec: exec, Role: config.RoleConfig{Model: "synthmodel"}}
	result, _, err := s.Synthesize(context.Background(), "tarea", []string{"candidata 1", "candidata 2"})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if result.Winner != 2 || result.Content != "candidata 2" {
		t.Fatalf("got %+v", result)
	}
}

func TestExecutorSynthesizer_WinnerOutOfRange_ReturnsError(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(mock.New(mock.Config{Name: "synthmodel", Response: `{"winner": 5, "reasoning": "x"}`}))
	models := &config.ModelsFile{Models: map[string]config.ModelConfig{"synthmodel": {Provider: "synthmodel"}}}
	exec := execution.New(reg, models, map[string]execution.ProviderPolicy{}, nil)

	s := &ExecutorSynthesizer{Exec: exec, Role: config.RoleConfig{Model: "synthmodel"}}
	if _, _, err := s.Synthesize(context.Background(), "tarea", []string{"a", "b"}); err == nil {
		t.Fatal("esperaba error por winner fuera de rango")
	}
}

func TestExecutorSynthesizer_InvalidJSON_ReturnsError(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(mock.New(mock.Config{Name: "synthmodel", Response: "no soy JSON"}))
	models := &config.ModelsFile{Models: map[string]config.ModelConfig{"synthmodel": {Provider: "synthmodel"}}}
	exec := execution.New(reg, models, map[string]execution.ProviderPolicy{}, nil)

	s := &ExecutorSynthesizer{Exec: exec, Role: config.RoleConfig{Model: "synthmodel"}}
	if _, _, err := s.Synthesize(context.Background(), "tarea", []string{"a", "b"}); err == nil {
		t.Fatal("esperaba error por JSON inválido")
	}
}
