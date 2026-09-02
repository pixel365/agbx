package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	validConfig    = "version: 1\n"
	versionCommand = "version"
)

func TestRootCommandAllowsMissingDefaultConfigFile(t *testing.T) {
	changeWorkingDirectory(t, t.TempDir())

	var out bytes.Buffer
	cmd := NewRootCommand(t.Context())
	cmd.SetArgs([]string{versionCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "dev\n", out.String())
}

func TestRootCommandLoadsExplicitConfigFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(validConfig), 0o600))

	var out bytes.Buffer
	cmd := NewRootCommand(t.Context())
	cmd.SetArgs([]string{"--config", filePath, versionCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "dev\n", out.String())
}

func TestRootCommandRejectsMissingExplicitConfigFile(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "missing.yaml")

	cmd := NewRootCommand(t.Context())
	cmd.SetArgs([]string{"--config", missingFile, versionCommand})
	cmd.SetErr(io.Discard)

	require.Error(t, cmd.Execute())
}

func changeWorkingDirectory(t *testing.T, directory string) {
	t.Helper()

	previousDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(directory))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(previousDirectory))
	})
}
