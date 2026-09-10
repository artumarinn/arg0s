package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

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

	t.Setenv("ARG0S_HOME", home)
	doctorOut, err := execRoot(t, "doctor")
	require.NoError(t, err, "doctor debe seguir en verde tras init --force repetido.\noutput:\n%s", doctorOut)
}

func TestDoctor_OnCleanMachine_FailsWithoutInit(t *testing.T) {
	home := t.TempDir() // existe pero vacío: ARG0S_HOME nunca inicializado
	t.Setenv("ARG0S_HOME", home)

	out, err := execRoot(t, "doctor")
	require.Error(t, err, "doctor sin init debe fallar (roles rotos, sin catálogo real).\noutput:\n%s", out)
	require.Contains(t, out, "✗")
}
