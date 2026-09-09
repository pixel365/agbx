package networklearn

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/cmd/internal/run"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	learnProviderName = "claude"
	learnConfig       = "version: 1\nimage:\n  name: example/image\n  tag: 1.0\n"
	learnFlows        = `{"log":{"entries":[` +
		`{"request":{"url":"https://github.com/"}},` +
		`{"request":{"url":"https://api.example.com/v1"}}]}}`
)

func TestLearnCommandPrintsSuggestedProviderPolicy(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(learnConfig), 0o600),
	)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(learnTestProvider{}))
	dockerClient := &learnDockerClient{hasImage: true}
	command := NewNetworkCommand(func() (run.DockerClient, error) {
		return dockerClient, nil
	}, providers)
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetArgs([]string{"learn", learnProviderName, "--prompt", "describe this project"})

	require.NoError(t, command.ExecuteContext(t.Context()))
	assert.Equal(
		t,
		[]string{learnProviderName, "--prompt", "describe this project"},
		dockerClient.request.Command,
	)
	require.NotNil(t, dockerClient.request.NetworkProxy)
	assert.Empty(t, dockerClient.request.NetworkProxy.PolicyScriptPath)
	assert.Contains(
		t,
		output.String(),
		"Observed network destinations:\n- api.example.com\n- github.com",
	)
	assert.Contains(t, output.String(), "providers:\n  claude:\n    network:\n      allow:")
	assert.True(t, dockerClient.closed)
}

func TestLearnCommandRejectsUnknownProvider(t *testing.T) {
	command := NewNetworkCommand(func() (run.DockerClient, error) {
		return &learnDockerClient{}, nil
	}, provider.NewRegistry())
	command.SetArgs([]string{"learn", "unknown"})

	err := command.ExecuteContext(t.Context())

	require.EqualError(t, err, `provider "unknown" is not supported`)
}

type learnDockerClient struct {
	request  docker.RunRequest
	hasImage bool
	closed   bool
}

func (c *learnDockerClient) HasImage(_ context.Context, _ string) (bool, error) {
	return c.hasImage, nil
}

func (c *learnDockerClient) Build(_ context.Context, _ docker.BuildRequest) error {
	return nil
}

func (c *learnDockerClient) Run(_ context.Context, request docker.RunRequest) error {
	c.request = request

	return os.WriteFile(
		filepath.Join(request.NetworkProxy.LogDirectory, flowsFileName),
		[]byte(learnFlows),
		0o600,
	)
}

func (c *learnDockerClient) Close() error {
	c.closed = true

	return nil
}

type learnTestProvider struct{}

func (learnTestProvider) Name() string {
	return learnProviderName
}

func (learnTestProvider) BuildRecipe(image config.Image) (provider.BuildRecipe, error) {
	return provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}, nil
}

func (learnTestProvider) Command(args []string, _ []config.Mount) ([]string, error) {
	return append([]string{learnProviderName}, args...), nil
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
