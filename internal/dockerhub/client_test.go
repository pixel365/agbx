package dockerhub

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const golangRepository = "golang"

func TestClientListsTagsFromAllPages(t *testing.T) {
	authServer := httptest.NewServer(
		http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			assert.Equal(t, registryService, request.URL.Query().Get("service"))
			assert.Equal(t, "repository:library/golang:pull", request.URL.Query().Get("scope"))
			_, _ = fmt.Fprint(response, `{"token":"test-token"}`)
		}),
	)
	defer authServer.Close()

	var registryServer *httptest.Server
	registryServer = httptest.NewServer(
		http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			assert.Equal(t, "/v2/library/golang/tags/list", request.URL.Path)
			assert.Equal(t, "100", request.URL.Query().Get("n"))
			assert.Equal(t, "Bearer test-token", request.Header.Get("Authorization"))

			if request.URL.Query().Get("last") == "latest" {
				_, _ = fmt.Fprint(response, `{"tags":["1.27"]}`)

				return
			}

			response.Header().Set(
				"Link",
				fmt.Sprintf(
					"<%s/v2/library/golang/tags/list?n=100&last=latest>; rel=\"next\"",
					registryServer.URL,
				),
			)
			_, _ = fmt.Fprint(response, `{"tags":["latest"]}`)
		}),
	)
	defer registryServer.Close()

	authURL, err := url.Parse(authServer.URL)
	require.NoError(t, err)
	registryURL, err := url.Parse(registryServer.URL)
	require.NoError(t, err)
	client := newClient(registryServer.Client(), authURL, registryURL)

	tags, err := client.ListTags(t.Context(), golangRepository)

	require.NoError(t, err)
	assert.Equal(t, []string{"latest", "1.27"}, tags)
}

func TestClientResolvesDigest(t *testing.T) {
	authServer := httptest.NewServer(
		http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			assert.Equal(t, "repository:library/golang:pull", request.URL.Query().Get("scope"))
			_, _ = fmt.Fprint(response, `{"token":"test-token"}`)
		}),
	)
	defer authServer.Close()

	registryServer := httptest.NewServer(
		http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			assert.Equal(t, http.MethodHead, request.Method)
			assert.Equal(t, "/v2/library/golang/manifests/1.27", request.URL.Path)
			assert.Equal(t, "Bearer test-token", request.Header.Get("Authorization"))
			assert.Equal(t, manifestAccept, request.Header.Get("Accept"))
			response.Header().Set(
				"Docker-Content-Digest",
				"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			)
		}),
	)
	defer registryServer.Close()

	authURL, err := url.Parse(authServer.URL)
	require.NoError(t, err)
	registryURL, err := url.Parse(registryServer.URL)
	require.NoError(t, err)
	client := newClient(registryServer.Client(), authURL, registryURL)

	digest, err := client.ResolveDigest(t.Context(), golangRepository, "1.27")

	require.NoError(t, err)
	assert.Equal(
		t,
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		digest,
	)
}

func TestRepositoryPath(t *testing.T) {
	testCases := []struct {
		name      string
		imageName string
		want      string
		wantError string
	}{
		{
			name:      "official image",
			imageName: golangRepository,
			want:      "library/" + golangRepository,
		},
		{
			name:      "namespaced image",
			imageName: "openai/codex",
			want:      "openai/codex",
		},
		{
			name:      "fully qualified official image",
			imageName: "docker.io/library/" + golangRepository,
			want:      "library/" + golangRepository,
		},
		{
			name:      "invalid image name",
			imageName: "invalid/image/name",
			wantError: "invalid Docker Hub image name \"invalid/image/name\"",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := repositoryPath(testCase.imageName)

			if testCase.wantError != "" {
				require.EqualError(t, err, testCase.wantError)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		})
	}
}
