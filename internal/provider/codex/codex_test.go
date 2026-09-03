package codex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
)

const (
	binaryName   = "agbx-codex"
	helpArgument = "--help"
)

func TestProviderBuildRecipe(t *testing.T) {
	image := config.Image{
		Name:   "example/image",
		Tag:    "1.0",
		Digest: "sha256:abc",
	}

	recipe, err := New().BuildRecipe(image)

	require.NoError(t, err)
	assert.Equal(t, dockerfile, recipe.Dockerfile)
	assert.Equal(t, map[string]string{baseImageBuildArg: image.Reference()}, recipe.BuildArgs)
}

func TestProviderCommand(t *testing.T) {
	command, err := New().Command([]string{helpArgument}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{binaryName, "--sandbox", "workspace-write", helpArgument}, command)
}

func TestProviderCommandAddsAdditionalMountDirectory(t *testing.T) {
	command, err := New().Command(
		[]string{helpArgument},
		[]config.Mount{{Source: "instructions", Target: config.AdditionalMountDirectory}},
	)

	require.NoError(t, err)
	assert.Equal(
		t,
		[]string{
			binaryName,
			"--sandbox",
			"workspace-write",
			"--add-dir",
			config.AdditionalMountDirectory,
			helpArgument,
		},
		command,
	)
}

func TestProviderCommandPassesAuthenticationCommand(t *testing.T) {
	command, err := New().Command([]string{loginCommand, "--device-auth"}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{binaryName, loginCommand, "--device-auth"}, command)
}

func TestProviderDockerfileUsesDeviceAuthentication(t *testing.T) {
	assert.Contains(t, dockerfile, "codex login status > /dev/null 2>&1")
	assert.Contains(t, dockerfile, "codex login --device-auth")
}
