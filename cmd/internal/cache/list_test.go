package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/preparedcache"
	"github.com/pixel365/agbx/internal/provider"
)

func TestListCommandListsProjectAndUnattributedImages(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	configFile := filepath.Join(directory, ".agbx.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(cacheConfig), 0o600))
	require.NoError(t, preparedcache.Record(configFile, cacheProvider, cacheHistoricalImage))

	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(cacheTestProvider{}))
	image := config.Image{Name: cacheImageName, Tag: cacheImageTag, Digest: cacheImageDigest}
	recipe := provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}
	currentImage := recipe.PreparedImageReference(cacheProvider, image)
	dockerClient := &recordingDockerClient{images: []docker.PreparedImage{
		{
			CreatedAt: time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC),
			ImageID:   "sha256:current",
			Reference: currentImage,
			Provider:  cacheProvider,
			Size:      1024,
			Tagged:    true,
		},
		{
			CreatedAt: time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC),
			ImageID:   "sha256:historical",
			Reference: cacheHistoricalImage,
			Provider:  cacheProvider,
			Size:      2048,
			Tagged:    true,
			Legacy:    true,
		},
		{
			CreatedAt: time.Date(2026, time.September, 8, 0, 0, 0, 0, time.UTC),
			ImageID:   "sha256:unattributed",
			Reference: cacheUnattributedImage,
			Provider:  "codex",
			Size:      4096,
			Tagged:    true,
		},
	}}
	command := NewListCommand(newDockerClient(dockerClient), providers)
	var output bytes.Buffer
	command.SetOut(&output)

	require.NoError(t, command.ExecuteContext(t.Context()))
	assert.Contains(t, output.String(), "Project: "+configFile)
	assert.Contains(t, output.String(), "Prepared images: 2")
	assert.Contains(t, output.String(), "current")
	assert.Contains(t, output.String(), currentImage)
	assert.Contains(t, output.String(), "historical, legacy")
	assert.Contains(t, output.String(), cacheHistoricalImage)
	assert.Contains(t, output.String(), "Unattributed prepared images:")
	assert.Contains(t, output.String(), "unattributed")
	assert.Contains(t, output.String(), cacheUnattributedImage)
	assert.True(t, dockerClient.closed)
}

func TestListCommandShowsEmptyProjectCache(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, ".agbx.yaml"), []byte(cacheConfig), 0o600),
	)
	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(cacheTestProvider{}))
	command := NewListCommand(newDockerClient(&recordingDockerClient{}), providers)
	var output bytes.Buffer
	command.SetOut(&output)

	require.NoError(t, command.ExecuteContext(t.Context()))
	assert.Contains(t, output.String(), "Prepared images: 0")
	assert.Contains(t, output.String(), "No prepared images are associated with this project.")
}

func TestClassifyImagesRecognizesCurrentImage(t *testing.T) {
	image := docker.PreparedImage{
		Reference: cacheHistoricalImage,
		Provider:  cacheProvider,
		Tagged:    true,
	}

	projectImages, unattributedImages := classifyImages(
		[]docker.PreparedImage{image},
		map[string]string{cacheHistoricalImage: cacheProvider},
		preparedcache.Project{},
	)

	assert.Len(t, projectImages, 1)
	assert.Equal(t, "current", projectImages[0].State)
	assert.Empty(t, unattributedImages)
}

func TestImageStateIncludesImageFlags(t *testing.T) {
	state := imageState(listedImage{Image: docker.PreparedImage{Legacy: true}, State: "historical"})

	assert.Equal(t, "historical, untagged, legacy", state)
}
