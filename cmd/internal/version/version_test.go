package version

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCommandPrintsVersion(t *testing.T) {
	restore := setVersionMetadata(t, "1.2.3", "abc123", "2026-08-03T00:00:00Z")
	defer restore()

	var out bytes.Buffer
	cmd := NewVersionCommand()
	cmd.SetOut(&out)

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "1.2.3\n", out.String())
}

func TestVersionCommandPrintsVerboseMetadata(t *testing.T) {
	restore := setVersionMetadata(t, "1.2.3", "abc123", "2026-08-03T00:00:00Z")
	defer restore()

	var out bytes.Buffer
	cmd := NewVersionCommand()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--verbose"})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, "Version: 1.2.3\n"+
		"Commit: abc123\n"+
		"Release Date: 2026-08-03T00:00:00Z\n"+
		"Go Version: "+runtime.Version()+"\n"+
		"Platform: "+runtime.GOOS+"/"+runtime.GOARCH+"\n", out.String())
}

func setVersionMetadata(
	t *testing.T,
	nextVersion string,
	nextCommit string,
	nextReleaseDate string,
) func() {
	t.Helper()

	previousVersion := version
	previousCommit := commit
	previousReleaseDate := releaseDate

	version = nextVersion
	commit = nextCommit
	releaseDate = nextReleaseDate

	return func() {
		version = previousVersion
		commit = previousCommit
		releaseDate = previousReleaseDate
	}
}
