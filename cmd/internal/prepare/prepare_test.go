package prepare

import (
	"bytes"
	"context"
	"io"
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

func TestPrepareCommandRejectsUnsupportedProvider(t *testing.T) {
	cmd := NewPrepareCommand(newDockerClient(&recordingDockerClient{}), provider.NewRegistry())
	cmd.SetArgs([]string{providerName})

	err := cmd.Execute()

	require.Error(t, err)
	assert.EqualError(t, err, "provider \"claude\" is not supported")
}

func TestPrepareCommandRequiresProvider(t *testing.T) {
	cmd := NewPrepareCommand(newDockerClient(&recordingDockerClient{}), provider.NewRegistry())
	cmd.SetArgs(nil)

	require.Error(t, cmd.Execute())
}

func TestPrepareCommandBuildsRegisteredProvider(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	providers := provider.NewRegistry()
	selectedProvider := &testProvider{}
	require.NoError(t, providers.Register(selectedProvider))
	dockerClient := &recordingDockerClient{hasImage: false}
	cmd := NewPrepareCommand(newDockerClient(dockerClient), providers)
	cmd.SetArgs([]string{providerName})
	var out bytes.Buffer
	cmd.SetOut(&out)

	err := cmd.Execute()

	require.NoError(t, err)
	expectedImage := config.Image{
		Name:   "example/image",
		Tag:    "1.0",
		Digest: "sha256:abc",
	}
	expectedRecipe := provider.BuildRecipe{
		Dockerfile: "FROM " + expectedImage.Reference(),
		BuildArgs:  map[string]string{"BASE_IMAGE": expectedImage.Reference()},
	}
	assert.Equal(t, expectedImage, selectedProvider.image)
	assert.Equal(t, expectedRecipe.Dockerfile, dockerClient.request.Dockerfile)
	assert.Equal(t, expectedRecipe.BuildArgs, dockerClient.request.BuildArgs)
	assert.Equal(
		t,
		expectedRecipe.PreparedImageReference(providerName, expectedImage),
		dockerClient.request.Tag,
	)
	assert.True(t, dockerClient.closed)
	assert.Equal(t, dockerClient.request.Tag, dockerClient.inspectedImage)
	assert.Equal(t, "Prepared provider image: "+dockerClient.request.Tag+"\n", out.String())
}

func TestPrepareCommandSkipsExistingImage(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(&testProvider{}))
	dockerClient := &recordingDockerClient{hasImage: true}
	cmd := NewPrepareCommand(newDockerClient(dockerClient), providers)
	cmd.SetArgs([]string{providerName})
	var out bytes.Buffer
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Empty(t, dockerClient.request)
	assert.Equal(
		t,
		"Provider image is already prepared: "+dockerClient.inspectedImage+"\n",
		out.String(),
	)
}

func TestPrepareCommandForceBuildsExistingImage(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(&testProvider{}))
	dockerClient := &recordingDockerClient{hasImage: true}
	cmd := NewPrepareCommand(newDockerClient(dockerClient), providers)
	cmd.SetArgs([]string{"--force", providerName})
	cmd.SetOut(io.Discard)

	require.NoError(t, cmd.Execute())
	assert.NotEmpty(t, dockerClient.request.Tag)
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

	return provider.BuildRecipe{
		Dockerfile: "FROM " + image.Reference(),
		BuildArgs:  map[string]string{"BASE_IMAGE": image.Reference()},
	}, nil
}

func (*testProvider) Command([]string, []config.Mount) ([]string, error) {
	return nil, nil
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

type recordingDockerClient struct {
	request        docker.BuildRequest
	inspectedImage string
	hasImage       bool
	closed         bool
}

func (client *recordingDockerClient) HasImage(_ context.Context, image string) (bool, error) {
	client.inspectedImage = image

	return client.hasImage, nil
}

func (client *recordingDockerClient) Build(_ context.Context, request docker.BuildRequest) error {
	client.request = request

	return nil
}

func (client *recordingDockerClient) Close() error {
	client.closed = true

	return nil
}

func newDockerClient(client DockerClient) DockerClientFunc {
	return func() (DockerClient, error) {
		return client, nil
	}
}
