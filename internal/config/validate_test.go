package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/artumarinn/arg0s/internal/core"
)

func findCheck(checks []core.Check, group, name string) (core.Check, bool) {
	for _, c := range checks {
		if c.Group == group && c.Name == name {
			return c, true
		}
	}
	return core.Check{}, false
}

func TestCheckRoles_ModelNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARG0S_HOME", home)

	cfg := DefaultConfig()
	models := &ModelsFile{Models: map[string]ModelConfig{}} // catálogo vacío

	checks := checkRoles(cfg, models)
	c, ok := findCheck(checks, "Roles", "generator")
	require.True(t, ok)
	require.Equal(t, core.CheckFail, c.Status)
	require.Contains(t, c.Detail, "model not found")
}

func TestCheckRoles_ProviderDisabled(t *testing.T) {
	cfg := DefaultConfig()
	p := cfg.Providers["gemini"]
	p.Enabled = false
	cfg.Providers["gemini"] = p

	models := &ModelsFile{Models: map[string]ModelConfig{
		"gemini-flash": {Provider: "gemini"},
	}}

	checks := checkRoles(cfg, models)
	c, ok := findCheck(checks, "Roles", "generator")
	require.True(t, ok)
	require.Equal(t, core.CheckFail, c.Status)
	require.Contains(t, c.Detail, "disabled")
}

func TestCheckRoles_Pass(t *testing.T) {
	cfg := DefaultConfig()
	models := &ModelsFile{Models: map[string]ModelConfig{
		"gemini-flash": {Provider: "gemini"},
	}}

	checks := checkRoles(cfg, models)
	c, ok := findCheck(checks, "Roles", "generator")
	require.True(t, ok)
	require.Equal(t, core.CheckPass, c.Status)
}

func TestCheckLimits(t *testing.T) {
	cfg := DefaultConfig()
	checks := checkLimits(cfg)
	require.Equal(t, core.CheckPass, checks[0].Status)

	cfg.Limits.PerTaskCostUSD = cfg.Limits.DailyCostUSD + 1
	checks = checkLimits(cfg)
	require.Equal(t, core.CheckFail, checks[0].Status)
}

func TestCheckProviders_MissingAPIKeyIsSkipNotFail(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARG0S_HOME", home)
	t.Setenv("GEMINI_API_KEY", "")

	cfg := DefaultConfig()
	checks := checkProviders(cfg)
	c, ok := findCheck(checks, "Providers", "gemini")
	require.True(t, ok)
	require.Equal(t, core.CheckSkip, c.Status)
}

func TestCheckProviders_KeyPresentInEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARG0S_HOME", home)
	t.Setenv("GEMINI_API_KEY", "sk-test")

	cfg := DefaultConfig()
	checks := checkProviders(cfg)
	c, ok := findCheck(checks, "Providers", "gemini")
	require.True(t, ok)
	require.Equal(t, core.CheckPass, c.Status)
}

func TestCheckProviders_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	checks := checkProviders(cfg)
	c, ok := findCheck(checks, "Providers", "openrouter")
	require.True(t, ok)
	require.Equal(t, core.CheckSkip, c.Status)
}
