package config

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
