package check

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/provider"
)

const validConfig = "version: 1\nimage:\n  name: example/image\n  tag: 1.0\n  digest: sha256:abc\n"

func TestCheckCommandPrintsStandardOutputWithoutVerboseStatus(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	dockerClient := &recordingDockerClient{}
	cmd := NewCheckCommand(newDockerClient(dockerClient), provider.NewRegistry())
	var out bytes.Buffer
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Configuration is valid.\nDocker daemon is available.\n", out.String())
	assert.Empty(t, dockerClient.inspectedImages)
	assert.True(t, dockerClient.closed)
}

func TestCheckCommandPrintsAllProviderStatusesInVerboseMode(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	image := config.Image{Name: "example/image", Tag: "1.0", Digest: "sha256:abc"}
	providers := provider.NewRegistry()
	claudeProvider := testProvider{name: "claude"}
	codexProvider := testProvider{name: "codex"}
	require.NoError(t, providers.Register(codexProvider))
	require.NoError(t, providers.Register(claudeProvider))
	claudeImage := preparedImageReference(claudeProvider.Name(), image)
	codexImage := preparedImageReference(codexProvider.Name(), image)
	dockerClient := &recordingDockerClient{images: map[string]bool{claudeImage: true}}
	cmd := NewCheckCommand(newDockerClient(dockerClient), providers)
	cmd.SetArgs([]string{"--verbose"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{claudeImage, codexImage}, dockerClient.inspectedImages)
	assert.Equal(
		t,
		"Configuration is valid.\nDocker daemon is available.\n\n"+
			"Provider: claude\nImage: "+claudeImage+"\nStatus: prepared\n\n"+
			"Provider: codex\nImage: "+codexImage+"\nStatus: not prepared\n",
		out.String(),
	)
	assert.True(t, dockerClient.closed)
}

func TestCheckCommandReturnsImageCheckError(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(validConfig), 0o600),
	)

	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(testProvider{name: "claude"}))
	dockerClient := &recordingDockerClient{imageErr: errors.New("inspect failed")}
	cmd := NewCheckCommand(newDockerClient(dockerClient), providers)
	cmd.SetArgs([]string{"--verbose"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "check prepared image")
	assert.True(t, dockerClient.closed)
}

type testProvider struct {
	name string
}

func (provider testProvider) Name() string {
	return provider.name
}

func (testProvider) BuildRecipe(image config.Image) (provider.BuildRecipe, error) {
	return provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}, nil
}

func (testProvider) Command([]string, []config.Mount) ([]string, error) {
	return nil, nil
}

type recordingDockerClient struct {
	imageErr        error
	images          map[string]bool
	inspectedImages []string
	closed          bool
}

func (client *recordingDockerClient) Ping(context.Context) error {
	return nil
}

func (client *recordingDockerClient) HasImage(_ context.Context, image string) (bool, error) {
	client.inspectedImages = append(client.inspectedImages, image)
	if client.imageErr != nil {
		return false, client.imageErr
	}

	return client.images[image], nil
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

func preparedImageReference(providerName string, image config.Image) string {
	recipe := provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}

	return recipe.PreparedImageReference(providerName, image)
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
