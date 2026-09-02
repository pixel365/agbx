package prepare

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	providerName = "claude"
	validConfig  = "version: 1\nimage:\n  name: example/image\n  tag: 1.0\n  digest: sha256:abc\n"
)

func TestPrepareCommandRejectsUnsupportedProvider(t *testing.T) {
	cmd := NewPrepareCommand(provider.NewRegistry())
	cmd.SetArgs([]string{providerName})

	err := cmd.Execute()

	require.Error(t, err)
	assert.EqualError(t, err, "provider \"claude\" is not supported")
}

func TestPrepareCommandRequiresProvider(t *testing.T) {
	cmd := NewPrepareCommand(provider.NewRegistry())
	cmd.SetArgs(nil)

	require.Error(t, cmd.Execute())
}

func TestPrepareCommandRejectsUnimplementedRegisteredProvider(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	providers := provider.NewRegistry()
	selectedProvider := &testProvider{}
	require.NoError(t, providers.Register(selectedProvider))
	cmd := NewPrepareCommand(providers)
	cmd.SetArgs([]string{providerName})

	err := cmd.Execute()

	require.Error(t, err)
	require.EqualError(t, err, "build for provider \"claude\" is not implemented")
	assert.Equal(t, config.Image{
		Name:   "example/image",
		Tag:    "1.0",
		Digest: "sha256:abc",
	}, selectedProvider.image)
}

type testProvider struct {
	image config.Image
}

func (*testProvider) Name() string {
	return providerName
}

func (testProviderInstance *testProvider) BuildRecipe(
	image config.Image,
) (provider.BuildRecipe, error) {
	testProviderInstance.image = image

	return provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}, nil
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
