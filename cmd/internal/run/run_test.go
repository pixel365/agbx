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
	changeWorkingDirectory(t, directory)
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
	assert.Equal(t, "example/image:1.0@sha256:abc", dockerClient.request.Image)
	assert.Equal(t, directory, dockerClient.request.WorkingDirectory)
	assert.NotNil(t, dockerClient.request.Input)
	assert.NotNil(t, dockerClient.request.Output)
	assert.True(t, dockerClient.closed)
}

type recordingDockerClient struct {
	request docker.RunRequest
	closed  bool
}

type testProvider struct{}

func (testProvider) Name() string {
	return providerName
}

func (testProvider) BuildRecipe(config.Image) (provider.BuildRecipe, error) {
	return provider.BuildRecipe{}, nil
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
