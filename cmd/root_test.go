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
	checkCommand   = "check"
	configFlag     = "--config"
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
	cmd.SetArgs([]string{configFlag, filePath, versionCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "dev\n", out.String())
}

func TestRootCommandRejectsMissingExplicitConfigFile(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "missing.yaml")

	cmd := NewRootCommand(t.Context())
	cmd.SetArgs([]string{configFlag, missingFile, versionCommand})
	cmd.SetErr(io.Discard)

	require.Error(t, cmd.Execute())
}

func TestRootCommandChecksDefaultConfigFile(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	var out bytes.Buffer
	cmd := NewRootCommand(t.Context())
	cmd.SetArgs([]string{checkCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Configuration is valid.\n", out.String())
}

func TestRootCommandChecksExplicitConfigFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(validConfig), 0o600))

	var out bytes.Buffer
	cmd := NewRootCommand(t.Context())
	cmd.SetArgs([]string{configFlag, filePath, checkCommand})
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Configuration is valid.\n", out.String())
}

func TestRootCommandRejectsMissingDefaultConfigFile(t *testing.T) {
	changeWorkingDirectory(t, t.TempDir())

	cmd := NewRootCommand(t.Context())
	cmd.SetArgs([]string{checkCommand})
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
