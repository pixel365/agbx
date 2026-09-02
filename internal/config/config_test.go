package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	exampleImageName = "example/image"
	validConfigYAML  = "version: 1\nimage:\n  name: " + exampleImageName + "\n  tag: latest\n"
)

var validConfig = Config{
	Version: currentVersion,
	Image: Image{
		Name: exampleImageName,
		Tag:  "latest",
	},
}

func TestLoadReadsFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), defaultYAMLFileName)
	require.NoError(t, os.WriteFile(filePath, []byte(validConfigYAML), 0o600))

	configuration, err := Load(filePath)

	require.NoError(t, err)
	assert.Equal(t, validConfig, configuration)
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
		os.WriteFile(filepath.Join(directory, defaultYAMLFileName), []byte(validConfigYAML), 0o600),
	)

	configuration, err := LoadDefault(directory)

	require.NoError(t, err)
	assert.Equal(t, validConfig, configuration)
}

func TestLoadDefaultReadsYMLFile(t *testing.T) {
	directory := t.TempDir()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, defaultYMLFileName), []byte(validConfigYAML), 0o600),
	)

	configuration, err := LoadDefault(directory)

	require.NoError(t, err)
	assert.Equal(t, validConfig, configuration)
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
	configuration := validConfig
	configuration.Version++
	err := configuration.Validate()

	assert.EqualError(t, err, "unsupported config version 2")
}

func TestConfigValidateRequiresImageName(t *testing.T) {
	configuration := validConfig
	configuration.Image.Name = ""

	err := configuration.Validate()

	assert.EqualError(t, err, "config image name is required")
}

func TestConfigValidateRequiresImageTag(t *testing.T) {
	configuration := validConfig
	configuration.Image.Tag = ""

	err := configuration.Validate()

	assert.EqualError(t, err, "config image tag is required")
}

func TestImageReference(t *testing.T) {
	assert.Equal(t, exampleImageName+":1.0", Image{
		Name: exampleImageName,
		Tag:  "1.0",
	}.Reference())
	assert.Equal(t, exampleImageName+":1.0@sha256:abc", Image{
		Name:   exampleImageName,
		Tag:    "1.0",
		Digest: "sha256:abc",
	}.Reference())
}

func TestCreateWritesValidConfig(t *testing.T) {
	directory := t.TempDir()

	require.NoError(t, Create(directory, validConfig))

	configuration, err := Load(filepath.Join(directory, defaultYAMLFileName))
	require.NoError(t, err)
	assert.Equal(t, validConfig, configuration)
}
