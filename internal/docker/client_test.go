package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	mobyclient "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDockerContext     = "desktop-linux"
	testDockerContextHost = "unix:///tmp/docker.sock"
)

func TestClientHasImageReturnsFalseForMissingImage(t *testing.T) {
	dockerServer := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		http.Error(response, "No such image", http.StatusNotFound)
	}))
	t.Cleanup(dockerServer.Close)

	api, err := mobyclient.New(mobyclient.WithHost(dockerServer.URL))
	require.NoError(t, err)
	client := Client{api: api}

	hasImage, err := client.HasImage(t.Context(), "missing")

	require.NoError(t, err)
	assert.False(t, hasImage)
}

func TestDockerConfigDirectory(t *testing.T) {
	t.Run("uses Docker configuration environment variable", func(t *testing.T) {
		directory := t.TempDir()
		t.Setenv(dockerConfigEnvironmentVariable, directory)

		got, err := dockerConfigDirectory()

		require.NoError(t, err)
		assert.Equal(t, directory, got)
	})

	t.Run("uses default Docker configuration directory", func(t *testing.T) {
		t.Setenv(dockerConfigEnvironmentVariable, "")
		homeDirectory, err := os.UserHomeDir()
		require.NoError(t, err)

		got, err := dockerConfigDirectory()

		require.NoError(t, err)
		assert.Equal(t, filepath.Join(homeDirectory, ".docker"), got)
	})
}

func TestConfiguredContextName(t *testing.T) {
	testCases := []struct {
		name      string
		contents  string
		want      string
		wantError string
	}{
		{
			name: "missing configuration",
		},
		{
			name:     "configured context",
			contents: `{"currentContext":"` + testDockerContext + `"}`,
			want:     testDockerContext,
		},
		{
			name:      "invalid configuration",
			contents:  `{`,
			wantError: "parse Docker config file",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			directory := t.TempDir()
			if testCase.contents != "" {
				writeDockerConfig(t, directory, testCase.contents)
			}

			got, err := configuredContextName(directory)

			if testCase.wantError != "" {
				require.ErrorContains(t, err, testCase.wantError)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		})
	}
}

func TestContextHost(t *testing.T) {
	t.Run("missing metadata", func(t *testing.T) {
		_, err := contextHost(t.TempDir(), testDockerContext)

		require.ErrorContains(t, err, "read Docker context")
	})

	testCases := []struct {
		name      string
		contents  string
		want      string
		wantError string
	}{
		{
			name:     "Docker endpoint",
			contents: `{"Endpoints":{"docker":{"Host":"` + testDockerContextHost + `"}}}`,
			want:     testDockerContextHost,
		},
		{
			name:      "missing Docker endpoint",
			contents:  `{"Endpoints":{}}`,
			wantError: "has no Docker endpoint",
		},
		{
			name:      "Docker endpoint without host",
			contents:  `{"Endpoints":{"docker":{}}}`,
			wantError: "has no Docker endpoint",
		},
		{
			name:      "invalid metadata",
			contents:  `{`,
			wantError: "parse Docker context",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			directory := t.TempDir()
			writeDockerContextMetadata(t, directory, testDockerContext, testCase.contents)

			got, err := contextHost(directory, testDockerContext)

			if testCase.wantError != "" {
				require.ErrorContains(t, err, testCase.wantError)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		})
	}
}

func TestCurrentContextHost(t *testing.T) {
	testCases := []struct {
		name               string
		configuredContext  string
		environmentContext string
		contextHosts       map[string]string
		want               string
	}{
		{
			name: "missing configuration",
		},
		{
			name:              "default context",
			configuredContext: defaultDockerContext,
		},
		{
			name:               "default Docker context environment variable",
			configuredContext:  testDockerContext,
			environmentContext: defaultDockerContext,
		},
		{
			name:              "configured context",
			configuredContext: testDockerContext,
			contextHosts: map[string]string{
				testDockerContext: testDockerContextHost,
			},
			want: testDockerContextHost,
		},
		{
			name:               "Docker context environment variable takes precedence",
			configuredContext:  "ignored",
			environmentContext: testDockerContext,
			contextHosts: map[string]string{
				testDockerContext: testDockerContextHost,
			},
			want: testDockerContextHost,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Setenv(dockerConfigEnvironmentVariable, directory)
			t.Setenv(dockerContextEnvironmentVariable, testCase.environmentContext)
			if testCase.configuredContext != "" {
				writeDockerConfig(
					t,
					directory,
					`{"currentContext":"`+testCase.configuredContext+`"}`,
				)
			}
			for contextName, host := range testCase.contextHosts {
				writeDockerContextMetadata(
					t,
					directory,
					contextName,
					`{"Endpoints":{"docker":{"Host":"`+host+`"}}}`,
				)
			}

			got, err := currentContextHost()

			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		})
	}
}

func writeDockerConfig(t *testing.T, directory string, contents string) {
	t.Helper()
	require.NoError(
		t,
		os.WriteFile(filepath.Join(directory, "config.json"), []byte(contents), 0o600),
	)
}

func writeDockerContextMetadata(
	t *testing.T,
	directory string,
	contextName string,
	contents string,
) {
	t.Helper()
	contextHash := sha256.Sum256([]byte(contextName))
	metadataFile := filepath.Join(
		directory,
		"contexts",
		"meta",
		hex.EncodeToString(contextHash[:]),
		"meta.json",
	)
	require.NoError(t, os.MkdirAll(filepath.Dir(metadataFile), 0o700))
	require.NoError(t, os.WriteFile(metadataFile, []byte(contents), 0o600))
}
