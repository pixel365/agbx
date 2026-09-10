package preparedcache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	cacheProviderName = "claude"
	cacheFirstImage   = "agbx/prepared-claude:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cacheSecondImage  = "agbx/prepared-claude:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestRecordTracksCurrentAndHistoricalImages(t *testing.T) {
	directory := t.TempDir()
	configFile := filepath.Join(directory, ".agbx.yaml")
	require.NoError(t, os.WriteFile(configFile, nil, 0o600))
	t.Setenv(stateHomeEnvironmentVariable, t.TempDir())

	require.NoError(t, Record(configFile, cacheProviderName, cacheFirstImage))
	require.NoError(t, Record(configFile, cacheProviderName, cacheSecondImage))

	index, err := Load(configFile)

	require.NoError(t, err)
	provider := index.Providers[cacheProviderName]
	assert.Equal(t, cacheSecondImage, provider.CurrentImage)
	assert.Len(t, provider.Images, 2)
	assert.False(t, provider.Images[cacheFirstImage].FirstSelectedAt.IsZero())
	assert.False(t, provider.Images[cacheSecondImage].LastSelectedAt.IsZero())
	indexPath, err := projectIndexPath(configFile)
	require.NoError(t, err)
	info, err := os.Stat(indexPath)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestLoadReturnsEmptyProjectForMissingIndex(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), ".agbx.yaml")
	require.NoError(t, os.WriteFile(configFile, nil, 0o600))
	t.Setenv(stateHomeEnvironmentVariable, t.TempDir())

	index, err := Load(configFile)

	require.NoError(t, err)
	assert.Equal(t, indexVersion, index.Version)
	assert.Empty(t, index.Providers)
}

func TestAllImageReferencesCollectsProjectIndexes(t *testing.T) {
	directory := t.TempDir()
	firstConfigFile := filepath.Join(directory, "first.yaml")
	secondConfigFile := filepath.Join(directory, "second.yaml")
	require.NoError(t, os.WriteFile(firstConfigFile, nil, 0o600))
	require.NoError(t, os.WriteFile(secondConfigFile, nil, 0o600))
	t.Setenv(stateHomeEnvironmentVariable, t.TempDir())

	require.NoError(t, Record(firstConfigFile, cacheProviderName, cacheFirstImage))
	require.NoError(t, Record(firstConfigFile, cacheProviderName, cacheSecondImage))
	require.NoError(t, Record(secondConfigFile, cacheProviderName, cacheFirstImage))

	references, err := AllImageReferences()

	require.NoError(t, err)
	assert.Contains(t, references.Historical, cacheFirstImage)
	assert.Contains(t, references.Historical, cacheSecondImage)
	assert.Contains(t, references.Current, cacheFirstImage)
	assert.Contains(t, references.Current, cacheSecondImage)
}
