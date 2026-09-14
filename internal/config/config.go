// Package config carga y valida config.yaml y models.yaml de Arg0s.
// Precedencia (menor a mayor): defaults compilados → ~/.arg0s/config.yaml
// → ./.arg0s.yaml → variables de entorno ARG0S_* → flags de CLI.
// Ver docs/ARG0S.md sección 6.2.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type GeneralConfig struct {
	DefaultStrategy string `yaml:"default_strategy"`
	Theme           string `yaml:"theme"`
	LogLevel        string `yaml:"log_level"`
	Telemetry       bool   `yaml:"telemetry"`
	Editor          string `yaml:"editor"`
}

type DaemonConfig struct {
	Socket       string `yaml:"socket"`
	Autostart    bool   `yaml:"autostart"`
	IdleShutdown string `yaml:"idle_shutdown"`
}

type TierConfig struct {
	Models     []string `yaml:"models"`
	MaxCostUSD float64  `yaml:"max_cost_usd"`
}

// OverrideWhen son las condiciones de un override -- todos los campos
// no vacíos deben matchear el TaskProfile para que el override aplique
// (sección 8).
type OverrideWhen struct {
	Type       string `yaml:"type,omitempty"`
	Complexity string `yaml:"complexity,omitempty"`
	Privacy    string `yaml:"privacy,omitempty"`
}

type OverrideConfig struct {
	When          OverrideWhen `yaml:"when"`
	ForceTier     string       `yaml:"force_tier,omitempty"`
	ForceLocal    bool         `yaml:"force_local,omitempty"`
	ForceStrategy string       `yaml:"force_strategy,omitempty"`
	ForceModel    string       `yaml:"force_model,omitempty"`
}

type RouterConfig struct {
	Mode                string                `yaml:"mode"`
	ClassifierThreshold float64               `yaml:"classifier_threshold"`
	Tiers               map[string]TierConfig `yaml:"tiers"`
	Overrides           []OverrideConfig      `yaml:"overrides,omitempty"`
}

type LimitsConfig struct {
	DailyCostUSD    float64 `yaml:"daily_cost_usd"`
	PerTaskCostUSD  float64 `yaml:"per_task_cost_usd"`
	PerTaskMaxCalls int     `yaml:"per_task_max_calls"`
	WarnAtPercent   int     `yaml:"warn_at_percent"`
	OnExceed        string  `yaml:"on_exceed"`
}

type DecayConfig struct {
	Enabled         bool   `yaml:"enabled"`
	RevalidateAfter string `yaml:"revalidate_after"`
}

// MemoryConfig es sección 12 -- ver internal/memory para el modelo y
// la interfaz MemoryStore.
type MemoryConfig struct {
	Enabled              bool        `yaml:"enabled"`
	Backend              string      `yaml:"backend"` // sqlite | engram (engram no implementado todavía)
	Scope                string      `yaml:"scope"`   // project | global | session
	MaxFragmentsPerQuery int         `yaml:"max_fragments_per_query"`
	MinRelevance         float64     `yaml:"min_relevance"`
	Decay                DecayConfig `yaml:"decay"`
	Categories           []string    `yaml:"categories,omitempty"`
}

// LSPServerConfig es una entrada de codegraph.servers -- comando y
// args para levantar el LSP de un lenguaje.
type LSPServerConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
}

// CodeGraphConfig es sección 13. Fase 3 solo implementa el backend lsp
// con gopls -- el resto de servers queda declarado pero sin cliente
// real (ver internal/codegraph).
type CodeGraphConfig struct {
	Enabled       bool                       `yaml:"enabled"`
	Backend       string                     `yaml:"backend"` // lsp | scip | none
	IndexOnOpen   bool                       `yaml:"index_on_open"`
	Watch         bool                       `yaml:"watch"`
	MaxFileSizeKB int                        `yaml:"max_file_size_kb"`
	Ignore        []string                   `yaml:"ignore,omitempty"`
	Servers       map[string]LSPServerConfig `yaml:"servers,omitempty"`
}

// DivergenceConfig es sección 10.2 -- método de medición de desacuerdo
// entre modelo primario y secundarios.
type DivergenceConfig struct {
	Method            string  `yaml:"method"` // lexical | semantic | judge | hybrid
	Threshold         float64 `yaml:"threshold"`
	EscalateThreshold float64 `yaml:"escalate_threshold"`
}

// SynthesisConfig es sección 10.3.
type SynthesisConfig struct {
	Mode string `yaml:"mode"` // select_best | merge | critique_and_merge
}

// FusionConfig es sección 10 -- ver internal/fusion. Adaptive es la
// palabra clave (sección 10.1): el default NO es correr todos los
// modelos siempre, es correr el primario y solo escalar si hace falta.
type FusionConfig struct {
	Enabled    bool             `yaml:"enabled"`
	Adaptive   bool             `yaml:"adaptive"`
	Models     []string         `yaml:"models"`
	MaxModels  int              `yaml:"max_models"`
	Parallel   bool             `yaml:"parallel"`
	Divergence DivergenceConfig `yaml:"divergence"`
	Synthesis  SynthesisConfig  `yaml:"synthesis"`
}

// ContextConfig es sección 9 -- ver internal/contextc para el Context
// Compiler. BudgetSplit son proporciones (0..1) que deben sumar ~1.0;
// no se valida acá, el compilador simplemente usa lo que haya.
type ContextConfig struct {
	DefaultBudgetTokens int                `yaml:"default_budget_tokens"`
	ReserveOutputTokens int                `yaml:"reserve_output_tokens"`
	BudgetSplit         map[string]float64 `yaml:"budget_split"`
	CompressWhenOver    bool               `yaml:"compress_when_over"`
	CompressionModel    string             `yaml:"compression_model"` // nombre de rol, ej "summarizer"
}

// Config es la raíz de config.yaml. Las secciones de subsistemas
// todavía no implementados (judgment, presets) se ignoran al parsear
// -- yaml.v3 no falla ante claves desconocidas -- y se agregan structs
// propios cuando su fase llegue.
type Config struct {
	Version   int                       `yaml:"version"`
	General   GeneralConfig             `yaml:"general"`
	Daemon    DaemonConfig              `yaml:"daemon"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	Runtimes  map[string]RuntimeConfig  `yaml:"runtimes"`
	Roles     RolesConfig               `yaml:"roles"`
	Router    RouterConfig              `yaml:"router"`
	Limits    LimitsConfig              `yaml:"limits"`
	Memory    MemoryConfig              `yaml:"memory"`
	CodeGraph CodeGraphConfig           `yaml:"codegraph"`
	Context   ContextConfig             `yaml:"context"`
	Fusion    FusionConfig              `yaml:"fusion"`
}

// Home devuelve ~/.arg0s, respetando ARG0S_HOME si está seteada.
func Home() (string, error) {
	if h := os.Getenv("ARG0S_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".arg0s"), nil
}

// Load aplica la precedencia completa y devuelve el Config resultante.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	home, err := Home()
	if err != nil {
		return nil, err
	}

	if err := mergeYAMLFile(cfg, filepath.Join(home, "config.yaml")); err != nil {
		return nil, fmt.Errorf("load %s/config.yaml: %w", home, err)
	}
	if err := mergeYAMLFile(cfg, ".arg0s.yaml"); err != nil {
		return nil, fmt.Errorf("load ./.arg0s.yaml: %w", err)
	}
	applyEnvOverrides(cfg)

	return cfg, nil
}

// mergeYAMLFile decodifica path sobre cfg si el archivo existe. Un
// archivo ausente no es error: esa capa de precedencia simplemente no
// aporta nada.
func mergeYAMLFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

// applyEnvOverrides aplica variables ARG0S_* de mayor precedencia que
// los archivos. Fase 0 solo cubre las que ya tienen consumidor real.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("ARG0S_LOG_LEVEL"); v != "" {
		cfg.General.LogLevel = v
	}
	if v := os.Getenv("ARG0S_DEFAULT_STRATEGY"); v != "" {
		cfg.General.DefaultStrategy = v
	}
}

// LoadModels parsea models.yaml desde ~/.arg0s/models.yaml.
func LoadModels() (*ModelsFile, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, "models.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ModelsFile{Version: 1, Models: map[string]ModelConfig{}}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var mf ModelsFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &mf, nil
}

// ResolveModel sigue un alias hasta el nombre real de modelo en el
// catálogo, o devuelve id sin cambios si no es un alias.
func (mf *ModelsFile) ResolveModel(id string) string {
	if real, ok := mf.Aliases[id]; ok {
		return real
	}
	return id
}
