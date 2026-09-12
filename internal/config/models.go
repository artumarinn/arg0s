package config

import "github.com/artumarinn/arg0s/internal/core"

// ModelConfig es una entrada del catálogo models.yaml. Separado de
// config.yaml porque cambia con otra frecuencia (ver sección 6.4).
type ModelConfig struct {
	Provider        string       `yaml:"provider"`
	ProviderModelID string       `yaml:"provider_model_id"`
	ContextWindow   int          `yaml:"context_window"`
	MaxOutputTokens int          `yaml:"max_output_tokens"`
	Cost            ModelCost    `yaml:"cost"`
	Capabilities    []string     `yaml:"capabilities,omitempty"`
	Strengths       []string     `yaml:"strengths,omitempty"`
	Tier            string       `yaml:"tier"`
	Local           bool         `yaml:"local"`
	Hardware        *HardwareReq `yaml:"hardware,omitempty"`
	Notes           string       `yaml:"notes,omitempty"`
}

type ModelCost struct {
	InputPer1MUSD  float64 `yaml:"input_per_1m_usd"`
	OutputPer1MUSD float64 `yaml:"output_per_1m_usd"`
}

type HardwareReq struct {
	MinRAMGB          int `yaml:"min_ram_gb"`
	RecommendedVRAMGB int `yaml:"recommended_vram_gb"`
}

// ModelsFile es la raíz de models.yaml.
type ModelsFile struct {
	Version int                    `yaml:"version"`
	Models  map[string]ModelConfig `yaml:"models"`
	Aliases map[string]string      `yaml:"aliases,omitempty"`
}

// Resolve sigue alias→modelo real y arma el core.Model completo. Único
// dueño de esta resolución -- execution y router lo consumen a través
// de este método (execution vía la interfaz ModelCatalog, que
// *ModelsFile satisface directamente); ninguno la reimplementa.
func (mf *ModelsFile) Resolve(id string) (core.Model, bool) {
	real := mf.ResolveModel(id)
	mc, ok := mf.Models[real]
	if !ok {
		return core.Model{}, false
	}
	return core.Model{
		ID:              real,
		Provider:        mc.Provider,
		ProviderModelID: mc.ProviderModelID,
		ContextWindow:   mc.ContextWindow,
		MaxOutputTokens: mc.MaxOutputTokens,
		CostInputPer1M:  mc.Cost.InputPer1MUSD,
		CostOutputPer1M: mc.Cost.OutputPer1MUSD,
		Capabilities:    mc.Capabilities,
		Strengths:       mc.Strengths,
		Tier:            mc.Tier,
		Local:           mc.Local,
	}, true
}
