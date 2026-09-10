package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/preparedcache"
	"github.com/pixel365/agbx/internal/provider"
)

func TestPruneCommandListsAndRemovesHistoricalImages(t *testing.T) {
	directory := t.TempDir()
	changeWorkingDirectory(t, directory)
	configFile := filepath.Join(directory, ".agbx.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(cacheConfig), 0o600))
	image := config.Image{Name: cacheImageName, Tag: cacheImageTag, Digest: cacheImageDigest}
	recipe := provider.BuildRecipe{Dockerfile: "FROM " + image.Reference()}
	currentImage := recipe.PreparedImageReference(cacheProvider, image)
	require.NoError(t, preparedcache.Record(configFile, cacheProvider, cacheHistoricalImage))
	require.NoError(t, preparedcache.Record(configFile, cacheProvider, currentImage))

	providers := provider.NewRegistry()
	require.NoError(t, providers.Register(cacheTestProvider{}))
	dockerClient := &recordingDockerClient{images: []docker.PreparedImage{
		{Reference: currentImage, Provider: cacheProvider, Tagged: true},
		{Reference: cacheHistoricalImage, Provider: cacheProvider, Tagged: true},
		{Reference: cacheUnattributedImage, Provider: "codex", Tagged: true},
	}}
	command := NewPruneCommand(newDockerClient(dockerClient), providers)
	var output bytes.Buffer
	command.SetOut(&output)

	require.NoError(t, command.ExecuteContext(t.Context()))
	assert.Contains(t, output.String(), "Historical images eligible for removal: 1")
	assert.Contains(t, output.String(), cacheHistoricalImage)
	assert.Contains(t, output.String(), "agbx cache prune --apply")
	assert.Empty(t, dockerClient.removed)

	command = NewPruneCommand(newDockerClient(dockerClient), providers)
	command.SetArgs([]string{"--apply"})
	command.SetOut(&output)
	require.NoError(t, command.ExecuteContext(t.Context()))
	assert.Equal(
		t,
		[]docker.PreparedImage{
			{Reference: cacheHistoricalImage, Provider: cacheProvider, Tagged: true},
		},
		dockerClient.removed,
	)
	assert.Contains(t, output.String(), "Removed prepared images: 1")
}

func TestPruneCandidatesKeepsImageCurrentForAnotherProject(t *testing.T) {
	image := docker.PreparedImage{
		Reference: cacheHistoricalImage,
		Provider:  cacheProvider,
		Tagged:    true,
	}

	candidates := pruneCandidates(
		[]docker.PreparedImage{image},
		preparedcache.ImageReferences{
			Current:    map[string]struct{}{cacheHistoricalImage: {}},
			Historical: map[string]struct{}{cacheHistoricalImage: {}},
		},
	)

	assert.Empty(t, candidates)
}
