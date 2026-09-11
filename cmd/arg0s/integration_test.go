package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// redirectGeminiToLoopback apunta gemini.base_url a un httptest.Server
// en vez de generativelanguage.googleapis.com (host real). El template
// default trae gemini.enabled:true -- si el test corre `doctor` tal
// cual, providers.Health() le pega de verdad (regla dura #8: go test
// nunca toca la red). Deshabilitar gemini directamente rompería los
// roles que lo usan (generator, judge_a, fix_agent, synthesizer) sin
// que eso sea un problema real -- por eso se redirige, no se apaga.
// Ollama queda con su base_url real (localhost:11434): si no hay nada
// escuchando ahí, "connection refused" es loopback, no red real.
func redirectGeminiToLoopback(t *testing.T, home string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]string{{"name": "models/gemini-2.5-flash"}}})
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(home, "config.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	const from = "base_url: https://generativelanguage.googleapis.com/v1beta"
	to := "base_url: " + srv.URL
	require.Contains(t, string(data), from, "el template de config.yaml cambió de forma inesperada")

	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), from, to, 1)), 0o600))
}

// execRoot corre el CLI real (newRootCmd) con args, capturando stdout y
// devolviendo el error de RunE. Nunca pasa por main() ni os.Exit.
func execRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestInit_ThenDoctor_GreenPath(t *testing.T) {
	home := t.TempDir()

	initOut, err := execRoot(t, "init", "--home", home)
	require.NoError(t, err, "init output:\n%s", initOut)
	require.Contains(t, initOut, home)
	redirectGeminiToLoopback(t, home)

	t.Setenv("ARG0S_HOME", home)
	doctorOut, err := execRoot(t, "doctor")
	require.NoError(t, err, "doctor debe salir 0 en camino verde tras init.\noutput:\n%s", doctorOut)
	require.NotContains(t, doctorOut, "✗", "no debe haber fallos tras init.\noutput:\n%s", doctorOut)
}

func TestInit_WithoutForce_OnExistingDir_Errors(t *testing.T) {
	home := t.TempDir()

	_, err := execRoot(t, "init", "--home", home)
	require.NoError(t, err)

	_, err = execRoot(t, "init", "--home", home)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ya existe")
}

func TestInit_WithForce_IsIdempotent(t *testing.T) {
	home := t.TempDir()

	_, err := execRoot(t, "init", "--home", home)
	require.NoError(t, err)

	_, err = execRoot(t, "init", "--home", home, "--force")
	require.NoError(t, err)

	_, err = execRoot(t, "init", "--home", home, "--force")
	require.NoError(t, err)
	redirectGeminiToLoopback(t, home)

	t.Setenv("ARG0S_HOME", home)
	doctorOut, err := execRoot(t, "doctor")
	require.NoError(t, err, "doctor debe seguir en verde tras init --force repetido.\noutput:\n%s", doctorOut)
}

func TestDoctor_OnCleanMachine_FailsWithoutInit(t *testing.T) {
	home := t.TempDir() // existe pero vacío: ARG0S_HOME nunca inicializado
	t.Setenv("ARG0S_HOME", home)

	// Sin esto, los defaults compilados (gemini enabled:true) hacen que
	// doctor golpee generativelanguage.googleapis.com de verdad -- regla
	// dura #8. No corrió init, así que no hay config.yaml que parchear:
	// se escribe uno mínimo.
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte("providers:\n  gemini:\n    enabled: false\n"), 0o600))

	out, err := execRoot(t, "doctor")
	require.Error(t, err, "doctor sin init debe fallar (roles rotos, sin catálogo real).\noutput:\n%s", out)
	require.Contains(t, out, "✗")
}
