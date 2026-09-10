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

type RouterConfig struct {
	Mode                string                `yaml:"mode"`
	ClassifierThreshold float64               `yaml:"classifier_threshold"`
	Tiers               map[string]TierConfig `yaml:"tiers"`
}

type LimitsConfig struct {
	DailyCostUSD    float64 `yaml:"daily_cost_usd"`
	PerTaskCostUSD  float64 `yaml:"per_task_cost_usd"`
	PerTaskMaxCalls int     `yaml:"per_task_max_calls"`
	WarnAtPercent   int     `yaml:"warn_at_percent"`
	OnExceed        string  `yaml:"on_exceed"`
}

// Config es la raíz de config.yaml. Solo modela las secciones que Fase 0
// necesita validar (general, daemon, providers, runtimes, roles, router
// básico, limits). Las secciones de subsistemas no implementados todavía
// (fusion, judgment, context, memory, codegraph, presets) se ignoran al
// parsear — yaml.v3 no falla ante claves desconocidas — y se agregan
// structs propios cuando su fase llegue.
type Config struct {
	Version   int                       `yaml:"version"`
	General   GeneralConfig             `yaml:"general"`
	Daemon    DaemonConfig              `yaml:"daemon"`
	Providers map[string]ProviderConfig `yaml:"providers"`
	Runtimes  map[string]RuntimeConfig  `yaml:"runtimes"`
	Roles     RolesConfig               `yaml:"roles"`
	Router    RouterConfig              `yaml:"router"`
	Limits    LimitsConfig              `yaml:"limits"`
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
