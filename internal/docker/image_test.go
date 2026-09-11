package docker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mobyclient "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pixel365/agbx/internal/preparedimage"
)

const (
	exampleImage          = "example/image"
	preparedImageHash     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	preparedImageDigest   = "sha256:" + preparedImageHash
	preparedClaudeImage   = "agbx/prepared-claude:" + preparedImageHash
	preparedCodexImage    = "agbx/prepared-codex:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	legacyPreparedImage   = "agbx/prepared-claude:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	preparedImageID       = "sha256:prepared"
	untaggedPreparedImage = "sha256:untagged"
)

func TestImageFromReference(t *testing.T) {
	testCases := []struct {
		want              Image
		name              string
		reference         string
		repositoryDigests []string
		wantOK            bool
	}{
		{
			name:      "tagged image",
			reference: exampleImage + ":1.2.3",
			repositoryDigests: []string{
				exampleImage + "@" + preparedImageDigest,
			},
			want: Image{
				Name:   exampleImage,
				Tag:    "1.2.3",
				Digest: preparedImageDigest,
			},
			wantOK: true,
		},
		{
			name:      "registry with port",
			reference: "localhost:5000/example/image:1.2.3",
			repositoryDigests: []string{
				"localhost:5000/example/image@" + preparedImageDigest,
			},
			want: Image{
				Name:   "localhost:5000/example/image",
				Tag:    "1.2.3",
				Digest: preparedImageDigest,
			},
			wantOK: true,
		},
		{
			name:      "latest without digest",
			reference: exampleImage + ":latest",
			repositoryDigests: []string{
				exampleImage + "@" + preparedImageDigest,
			},
			want: Image{
				Name: exampleImage,
				Tag:  "latest",
			},
			wantOK: true,
		},
		{
			name:      "tagged image with digest",
			reference: "golang:1.27.0-alpine3.24@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc",
			want: Image{
				Name:   "golang",
				Tag:    "1.27.0-alpine3.24",
				Digest: "sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc",
			},
			wantOK: true,
		},
		{
			name:      "untagged image",
			reference: exampleImage,
			wantOK:    false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, gotOK := imageFromReference(testCase.reference, testCase.repositoryDigests)

			if gotOK != testCase.wantOK {
				t.Fatalf("imageFromReference() returned ok = %t, want %t", gotOK, testCase.wantOK)
			}
			if got != testCase.want {
				t.Fatalf("imageFromReference() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

func TestClientListImages(t *testing.T) {
	client := newImageTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assertDockerAPIPath(t, request, "/images/json")
		assert.Empty(t, request.URL.Query().Get("all"))
		writeImageTestResponse(t, response, []map[string]any{
			{
				"Id": "sha256:images",
				"RepoTags": []string{
					"node:latest",
					"golang:1.27",
					"<none>:<none>",
				},
				"RepoDigests": []string{
					"golang@" + preparedImageDigest,
				},
			},
		})
	})

	images, err := client.ListImages(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []Image{
		{
			Name:   "golang",
			Tag:    "1.27",
			Digest: preparedImageDigest,
		},
		{Name: "node", Tag: "latest"},
	}, images)
}

func TestClientListPreparedImages(t *testing.T) {
	client := newImageTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assertDockerAPIPath(t, request, "/images/json")
		assert.Equal(t, "1", request.URL.Query().Get("all"))
		writeImageTestResponse(t, response, []map[string]any{
			{
				"Created": 1_789_948_800,
				"Id":      preparedImageID,
				"Labels":  preparedimage.Labels("claude", preparedClaudeImage),
				"RepoTags": []string{
					preparedClaudeImage,
				},
				"Size": 1_024,
			},
			{
				"Created": 1_789_862_400,
				"Id":      untaggedPreparedImage,
				"Labels":  preparedimage.Labels("codex", preparedCodexImage),
				"Size":    2_048,
			},
			{
				"Created":  1_789_776_000,
				"Id":       "sha256:legacy",
				"RepoTags": []string{legacyPreparedImage},
				"Size":     4_096,
			},
			{
				"Id":       "sha256:unrelated",
				"RepoTags": []string{"example/unrelated:latest"},
			},
		})
	})

	images, err := client.ListPreparedImages(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []PreparedImage{
		{
			CreatedAt: time.Unix(1_789_948_800, 0),
			ImageID:   preparedImageID,
			Reference: preparedClaudeImage,
			Provider:  "claude",
			Size:      1_024,
			Tagged:    true,
		},
		{
			CreatedAt: time.Unix(1_789_776_000, 0),
			ImageID:   "sha256:legacy",
			Reference: legacyPreparedImage,
			Provider:  "claude",
			Size:      4_096,
			Tagged:    true,
			Legacy:    true,
		},
		{
			CreatedAt: time.Unix(1_789_862_400, 0),
			ImageID:   untaggedPreparedImage,
			Reference: preparedCodexImage,
			Provider:  "codex",
			Size:      2_048,
		},
	}, images)
}

func TestClientRemovePreparedImage(t *testing.T) {
	testCases := []struct {
		name  string
		want  string
		image PreparedImage
	}{
		{
			name:  "tagged image",
			image: PreparedImage{Reference: preparedClaudeImage, Tagged: true},
			want:  preparedClaudeImage,
		},
		{
			name:  "untagged image",
			image: PreparedImage{ImageID: untaggedPreparedImage, Reference: preparedCodexImage},
			want:  untaggedPreparedImage,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := newImageTestClient(
				t,
				func(response http.ResponseWriter, request *http.Request) {
					assert.Equal(t, http.MethodDelete, request.Method)
					assertDockerAPIPath(t, request, "/images/"+testCase.want)
					assert.Equal(t, "1", request.URL.Query().Get("noprune"))
					writeImageTestResponse(t, response, []map[string]string{})
				},
			)

			err := client.RemovePreparedImage(t.Context(), testCase.image)

			require.NoError(t, err)
		})
	}
}

func TestClientSearchImages(t *testing.T) {
	client := newImageTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assertDockerAPIPath(t, request, "/images/search")
		assert.Equal(t, "Go", request.URL.Query().Get("term"))
		writeImageTestResponse(t, response, []map[string]string{
			{"name": "zinc", "description": "Zinc image"},
			{"name": "", "description": "Ignored image"},
			{"name": "alpine", "description": "Alpine image"},
		})
	})

	images, err := client.SearchImages(t.Context(), "Go")

	require.NoError(t, err)
	assert.Equal(t, []SearchResult{
		{Name: "alpine", Description: "Alpine image"},
		{Name: "zinc", Description: "Zinc image"},
	}, images)
}

func newImageTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method == http.MethodHead && request.URL.Path == "/_ping" {
			response.Header().Set("API-Version", mobyclient.MaxAPIVersion)

			return
		}

		handler(response, request)
	}))
	t.Cleanup(server.Close)

	api, err := mobyclient.New(mobyclient.WithHost(server.URL))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, api.Close())
	})

	return &Client{api: api}
}

func writeImageTestResponse(t *testing.T, response http.ResponseWriter, value any) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Errorf("encode Docker API response: %v", err)
	}
}

func assertDockerAPIPath(t *testing.T, request *http.Request, endpoint string) {
	t.Helper()
	assert.True(t, strings.HasSuffix(request.URL.Path, endpoint), request.URL.Path)
}
