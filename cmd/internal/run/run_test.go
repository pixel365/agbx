package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	providerName = "claude"
	validConfig  = "version: 1\nimage:\n  name: example/image\n  tag: 1.0\n  digest: sha256:abc\n"
)

func TestRunCommandRunsConfiguredImage(t *testing.T) {
	directory := t.TempDir()
	stateHome := t.TempDir()
	changeWorkingDirectory(t, directory)
	t.Setenv(dataHomeEnvironmentVariable, stateHome)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	dockerClient := &recordingDockerClient{}
	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(testProvider{}))
	cmd := NewRunCommand(func() (DockerClient, error) {
		return dockerClient, nil
	}, providers)
	cmd.SetArgs([]string{providerName, "--", "--help"})

	require.NoError(t, cmd.ExecuteContext(t.Context()))
	assert.Equal(t, []string{providerName, "--help"}, dockerClient.request.Command)
	expectedImage := config.Image{
		Name:   "example/image",
		Tag:    "1.0",
		Digest: "sha256:abc",
	}
	expectedRecipe := provider.BuildRecipe{Dockerfile: "FROM " + expectedImage.Reference()}
	assert.Equal(
		t,
		expectedRecipe.PreparedImageReference(providerName, expectedImage),
		dockerClient.request.Image,
	)
	assert.Equal(t, directory, dockerClient.request.WorkingDirectory)
	assert.Equal(
		t,
		filepath.Join(stateHome, "agbx", "providers", providerName),
		dockerClient.request.StateDirectory,
	)
	assert.NotEmpty(t, dockerClient.request.User)
	assert.NotNil(t, dockerClient.request.Input)
	assert.NotNil(t, dockerClient.request.Output)
	assert.False(t, dockerClient.request.PullImage)
	assert.True(t, dockerClient.closed)
}

func TestProviderStateDirectoryUsesXDGDataHome(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv(dataHomeEnvironmentVariable, dataHome)

	directory, err := providerStateDirectory(providerName)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dataHome, "agbx", "providers", providerName), directory)
	info, err := os.Stat(directory)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestProviderStateDirectoryRejectsPath(t *testing.T) {
	for _, providerName := range []string{"../claude", "claude/provider", ".", ".."} {
		t.Run(providerName, func(t *testing.T) {
			_, err := providerStateDirectory(providerName)

			assert.Error(t, err)
		})
	}
}

type recordingDockerClient struct {
	request docker.RunRequest
	closed  bool
}

type testProvider struct{}

func (testProvider) Name() string {
	return providerName
}

func (testProvider) BuildRecipe(image config.Image) (provider.BuildRecipe, error) {
	return provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}, nil
}

func (testProvider) Command(args []string) ([]string, error) {
	return append([]string{providerName}, args...), nil
}

func (c *recordingDockerClient) Run(_ context.Context, request docker.RunRequest) error {
	c.request = request

	return nil
}

func (c *recordingDockerClient) Close() error {
	c.closed = true

	return nil
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
