package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSocketPath_DefaultExpandsAgainstHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARG0S_HOME", home)

	cfg := DefaultConfig()
	got, err := cfg.SocketPath()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, "daemon.sock"), got)
}

func TestSocketPath_AbsoluteOverridePassesThrough(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Daemon.Socket = "/tmp/custom.sock"
	got, err := cfg.SocketPath()
	require.NoError(t, err)
	require.Equal(t, "/tmp/custom.sock", got)
}

func TestLoad_DefaultsWhenNoFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARG0S_HOME", home)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "router", cfg.General.DefaultStrategy)
	require.True(t, cfg.Providers["gemini"].Enabled)
}

func TestLoad_FileOverridesDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARG0S_HOME", home)

	err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(`
general:
  default_strategy: direct
`), 0o600)
	require.NoError(t, err)

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "direct", cfg.General.DefaultStrategy)
	// Lo que no vino en el archivo sigue en su default compilado.
	require.Equal(t, "info", cfg.General.LogLevel)
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARG0S_HOME", home)

	err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(`
general:
  log_level: debug
`), 0o600)
	require.NoError(t, err)

	t.Setenv("ARG0S_LOG_LEVEL", "warn")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "warn", cfg.General.LogLevel)
}

func TestLoadModels_MissingFileReturnsEmptyCatalog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARG0S_HOME", home)

	mf, err := LoadModels()
	require.NoError(t, err)
	require.Empty(t, mf.Models)
}

func TestResolveModel_FollowsAlias(t *testing.T) {
	mf := &ModelsFile{Aliases: map[string]string{"fast": "qwen-coder-7b"}}
	require.Equal(t, "qwen-coder-7b", mf.ResolveModel("fast"))
	require.Equal(t, "gemini-pro", mf.ResolveModel("gemini-pro"))
}
