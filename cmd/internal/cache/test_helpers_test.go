package cache

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

const (
	cacheImageDigest = "sha256:abc"
	cacheImageName   = "example/image"
	cacheImageTag    = "1.0"
	cacheConfig      = "version: 1\nimage:\n  name: " + cacheImageName + "\n  tag: " + cacheImageTag +
		"\n  digest: " + cacheImageDigest + "\n"
	cacheProvider          = "claude"
	cacheHistoricalImage   = "agbx/prepared-claude:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	cacheUnattributedImage = "agbx/prepared-codex:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

type cacheTestProvider struct{}

func (cacheTestProvider) Name() string {
	return cacheProvider
}

func (cacheTestProvider) BuildRecipe(image config.Image) (provider.BuildRecipe, error) {
	return provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}, nil
}

func (cacheTestProvider) Command([]string, []config.Mount) ([]string, error) {
	return nil, nil
}

type recordingDockerClient struct {
	images  []docker.PreparedImage
	removed []docker.PreparedImage
	closed  bool
}

func (client *recordingDockerClient) ListPreparedImages(
	context.Context,
) ([]docker.PreparedImage, error) {
	return client.images, nil
}

func (client *recordingDockerClient) Close() error {
	client.closed = true

	return nil
}

func (client *recordingDockerClient) RemovePreparedImage(
	_ context.Context,
	image docker.PreparedImage,
) error {
	client.removed = append(client.removed, image)

	return nil
}

func newDockerClient(client DockerClient) DockerClientFunc {
	return func() (DockerClient, error) {
		return client, nil
	}
}

func changeWorkingDirectory(t *testing.T, directory string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	previousDirectory, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(directory))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(previousDirectory))
	})
}
