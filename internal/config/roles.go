package config

// RoleConfig define qué modelo cumple un rol del sistema (generator,
// judge_a, fix_agent, etc — ver sección 6.3 `roles:`).
type RoleConfig struct {
	Enabled     *bool   `yaml:"enabled,omitempty"` // nil = true (default)
	Model       string  `yaml:"model"`
	Fallback    string  `yaml:"fallback,omitempty"`
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens,omitempty"`
	Perspective string  `yaml:"perspective,omitempty"`
}

func (r RoleConfig) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

type RolesConfig struct {
	Classifier  RoleConfig `yaml:"classifier"`
	Generator   RoleConfig `yaml:"generator"`
	Synthesizer RoleConfig `yaml:"synthesizer"`
	JudgeA      RoleConfig `yaml:"judge_a"`
	JudgeB      RoleConfig `yaml:"judge_b"`
	JudgeC      RoleConfig `yaml:"judge_c"`
	FixAgent    RoleConfig `yaml:"fix_agent"`
	Summarizer  RoleConfig `yaml:"summarizer"`
}

// All devuelve los roles habilitados, con su nombre.
func (r RolesConfig) All() map[string]RoleConfig {
	m := map[string]RoleConfig{
		"classifier":  r.Classifier,
		"generator":   r.Generator,
		"synthesizer": r.Synthesizer,
		"judge_a":     r.JudgeA,
		"judge_b":     r.JudgeB,
		"judge_c":     r.JudgeC,
		"fix_agent":   r.FixAgent,
		"summarizer":  r.Summarizer,
	}
	for name, role := range m {
		if !role.IsEnabled() {
			delete(m, name)
		}
	}
	return m
}
