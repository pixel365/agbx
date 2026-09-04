package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/provider"
)

const helpArgument = "--help"

func TestProviderBuildRecipe(t *testing.T) {
	image := config.Image{
		Name:   "example/image",
		Tag:    "1.0",
		Digest: "sha256:abc",
	}

	recipe, err := New().BuildRecipe(image)

	require.NoError(t, err)
	assert.Equal(t, provider.NewBuildRecipe(image, dockerfile), recipe)
}

func TestProviderCommand(t *testing.T) {
	command, err := New().Command([]string{helpArgument}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"agbx-claude", helpArgument}, command)
}

func TestProviderCommandAddsAdditionalMountDirectory(t *testing.T) {
	command, err := New().Command(
		[]string{helpArgument},
		[]config.Mount{{Source: "instructions", Target: config.AdditionalMountDirectory}},
	)

	require.NoError(t, err)
	assert.Equal(
		t,
		[]string{"agbx-claude", "--add-dir", config.AdditionalMountDirectory, helpArgument},
		command,
	)
}
