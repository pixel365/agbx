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

func TestClientBearerToken(t *testing.T) {
	testCases := []struct {
		name      string
		body      string
		want      string
		wantError string
		status    int
	}{
		{
			name:   "access token",
			body:   `{"access_token":"test-token"}`,
			want:   "test-token",
			status: http.StatusOK,
		},
		{
			name:      "missing token",
			body:      `{}`,
			wantError: "has no token",
			status:    http.StatusOK,
		},
		{
			name:      "invalid response",
			body:      `{`,
			wantError: "decode Docker Hub token",
			status:    http.StatusOK,
		},
		{
			name:      "unexpected status",
			wantError: "unexpected response status 403 Forbidden",
			status:    http.StatusForbidden,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			authServer := httptest.NewServer(
				http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
					assert.Equal(t, http.MethodGet, request.Method)
					assert.Equal(t, registryService, request.URL.Query().Get("service"))
					assert.Equal(
						t,
						"repository:library/golang:pull",
						request.URL.Query().Get("scope"),
					)
					response.WriteHeader(testCase.status)
					_, _ = fmt.Fprint(response, testCase.body)
				}),
			)
			t.Cleanup(authServer.Close)

			authURL, err := url.Parse(authServer.URL)
			require.NoError(t, err)
			client := newClient(authServer.Client(), authURL, nil)

			token, err := client.bearerToken(t.Context(), "library/golang")

			if testCase.wantError != "" {
				require.ErrorContains(t, err, testCase.wantError)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, testCase.want, token)
		})
	}
}

func TestNextLink(t *testing.T) {
	testCases := []struct {
		name  string
		link  string
		want  string
		found bool
	}{
		{
			name:  "next link",
			link:  `</v2/library/golang/tags/list?last=latest>; rel="next"`,
			want:  "/v2/library/golang/tags/list?last=latest",
			found: true,
		},
		{
			name: "next link after another relation",
			link: `</v2/library/golang/tags/list?last=1.27>; rel="last", ` +
				`</v2/library/golang/tags/list?last=latest>; rel="next"`,
			want:  "/v2/library/golang/tags/list?last=latest",
			found: true,
		},
		{
			name: "no next link",
			link: `</v2/library/golang/tags/list?last=1.27>; rel="last"`,
		},
		{
			name:  "malformed next link",
			link:  `/v2/library/golang/tags/list?last=latest; rel="next"`,
			found: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, found := nextLink(testCase.link)

			assert.Equal(t, testCase.found, found)
			assert.Equal(t, testCase.want, got)
		})
	}
}

func TestNextPageURL(t *testing.T) {
	registryURL, err := url.Parse("https://registry.example.test")
	require.NoError(t, err)
	client := newClient(&http.Client{}, nil, registryURL)
	testCases := []struct {
		name      string
		link      string
		want      string
		wantError string
	}{
		{
			name: "no next link",
		},
		{
			name: "relative next link",
			link: `</v2/library/golang/tags/list?last=latest>; rel="next"`,
			want: "https://registry.example.test/v2/library/golang/tags/list?last=latest",
		},
		{
			name: "same origin absolute next link",
			link: `<https://registry.example.test/v2/library/golang/tags/list?last=latest>; rel="next"`,
			want: "https://registry.example.test/v2/library/golang/tags/list?last=latest",
		},
		{
			name:      "foreign origin",
			link:      `<https://unexpected.example.test/v2/tags/list>; rel="next"`,
			wantError: "unexpected origin",
		},
		{
			name:      "invalid URL",
			link:      `<http://[::1>; rel="next"`,
			wantError: "parse Docker Hub next page URL",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := client.nextPageURL(testCase.link)

			if testCase.wantError != "" {
				require.ErrorContains(t, err, testCase.wantError)

				return
			}
			require.NoError(t, err)
			if testCase.want == "" {
				assert.Nil(t, got)

				return
			}
			assert.Equal(t, testCase.want, got.String())
		})
	}
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
