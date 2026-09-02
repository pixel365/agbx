package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadReadsFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), defaultYAMLFileName)
	require.NoError(t, os.WriteFile(filePath, []byte(defaultContent), 0o600))

	configuration, err := Load(filePath)

	require.NoError(t, err)
	assert.Equal(t, Config{Version: currentVersion}, configuration)
}

func TestLoadReturnsErrorForMissingFile(t *testing.T) {
	configuration, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))

	assert.Equal(t, Config{}, configuration)
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestLoadReturnsErrorForDirectory(t *testing.T) {
	directory := t.TempDir()

	configuration, err := Load(directory)

	assert.Equal(t, Config{}, configuration)
	assert.Error(t, err)
}

func TestLoadDefaultReadsYAMLFile(t *testing.T) {
	directory := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, defaultYAMLFileName), []byte(defaultContent), 0o600),
	)

	configuration, err := LoadDefault(directory)

	require.NoError(t, err)
	assert.Equal(t, Config{Version: currentVersion}, configuration)
}

func TestLoadDefaultReadsYMLFile(t *testing.T) {
	directory := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, defaultYMLFileName), []byte(defaultContent), 0o600),
	)

	configuration, err := LoadDefault(directory)

	require.NoError(t, err)
	assert.Equal(t, Config{Version: currentVersion}, configuration)
}

func TestLoadDefaultReturnsNotFound(t *testing.T) {
	configuration, err := LoadDefault(t.TempDir())

	assert.Equal(t, Config{}, configuration)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestConfigValidateRequiresVersion(t *testing.T) {
	err := Config{}.Validate()

	assert.EqualError(t, err, "config version is required")
}

func TestConfigValidateRejectsUnsupportedVersion(t *testing.T) {
	err := Config{Version: currentVersion + 1}.Validate()

	assert.EqualError(t, err, "unsupported config version 2")
}
