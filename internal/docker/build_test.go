package docker

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildContextContainsDockerfile(t *testing.T) {
	buildContext, err := buildContext("FROM scratch\n")

	require.NoError(t, err)
	archive := tar.NewReader(buildContext)
	header, err := archive.Next()
	require.NoError(t, err)
	assert.Equal(t, dockerfileName, header.Name)
	assert.Equal(t, int64(0o600), header.Mode)

	dockerfile, err := io.ReadAll(archive)
	require.NoError(t, err)
	assert.Equal(t, "FROM scratch\n", string(dockerfile))

	_, err = archive.Next()
	assert.ErrorIs(t, err, io.EOF)
}

func TestDisplayBuildOutputWritesBuildStream(t *testing.T) {
	input := bytes.NewBufferString("{\"stream\":\"Step 1\\n\"}\n{\"stream\":\"Done\\n\"}\n")
	var output bytes.Buffer

	err := displayBuildOutput(input, &output)

	require.NoError(t, err)
	assert.Equal(t, "Step 1\nDone\n", output.String())
}
