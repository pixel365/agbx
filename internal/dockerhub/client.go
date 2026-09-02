package dockerhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	authHost        = "auth.docker.io"
	registryHost    = "registry-1.docker.io"
	registryService = "registry.docker.io"
	pageSize        = 100
	manifestAccept  = "application/vnd.oci.image.index.v1+json, " +
		"application/vnd.oci.image.manifest.v1+json, " +
		"application/vnd.docker.distribution.manifest.list.v2+json, " +
		"application/vnd.docker.distribution.manifest.v2+json"
)

type Client struct {
	httpClient  *http.Client
	authURL     *url.URL
	registryURL *url.URL
}

func NewClient() *Client {
	return newClient(
		&http.Client{Timeout: 10 * time.Second},
		&url.URL{Scheme: "https", Host: authHost},
		&url.URL{Scheme: "https", Host: registryHost},
	)
}

func newClient(httpClient *http.Client, authURL *url.URL, registryURL *url.URL) *Client {
	return &Client{
		httpClient:  httpClient,
		authURL:     authURL,
		registryURL: registryURL,
	}
}

func (c *Client) ListTags(ctx context.Context, imageName string) ([]string, error) {
	repository, err := repositoryPath(imageName)
	if err != nil {
		return nil, err
	}

	token, err := c.bearerToken(ctx, repository)
	if err != nil {
		return nil, err
	}

	pageURL := c.registryURL.JoinPath("v2", repository, "tags", "list")
	query := pageURL.Query()
	query.Set("n", strconv.Itoa(pageSize))
	pageURL.RawQuery = query.Encode()

	tags := make([]string, 0)
	for pageURL != nil {
		page, next, err := c.listTagsPage(ctx, pageURL, token)
		if err != nil {
			return nil, err
		}
		tags = append(tags, page.Tags...)
		pageURL, err = c.nextPageURL(next)
		if err != nil {
			return nil, err
		}
	}

	return tags, nil
}

func (c *Client) ResolveDigest(ctx context.Context, imageName string, tag string) (string, error) {
	repository, err := repositoryPath(imageName)
	if err != nil {
		return "", err
	}

	token, err := c.bearerToken(ctx, repository)
	if err != nil {
		return "", err
	}

	manifestURL := c.registryURL.JoinPath("v2", repository, "manifests", tag)
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	// #nosec G107 -- manifestURL uses Docker Hub's fixed registry origin and validated image data.
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, manifestURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create Docker Hub manifest request: %w", err)
	}
	request.Header.Set("Accept", manifestAccept)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("request Docker Hub manifest: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"request Docker Hub manifest: unexpected response status %s",
			response.Status,
		)
	}

	digest := response.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("manifest response from Docker Hub has no digest")
	}

	return digest, nil
}

func (c *Client) bearerToken(ctx context.Context, repository string) (string, error) {
	tokenURL := c.authURL.JoinPath("token")
	query := tokenURL.Query()
	query.Set("service", registryService)
	query.Set("scope", "repository:"+repository+":pull")
	tokenURL.RawQuery = query.Encode()

	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()

	// #nosec G107 -- tokenURL uses Docker Hub's fixed authorization origin.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create Docker Hub token request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("request Docker Hub token: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"request Docker Hub token: unexpected response status %s",
			response.Status,
		)
	}

	var token struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return "", fmt.Errorf("decode Docker Hub token: %w", err)
	}
	if token.Token != "" {
		return token.Token, nil
	}
	if token.AccessToken != "" {
		return token.AccessToken, nil
	}

	return "", fmt.Errorf("token response from Docker Hub has no token")
}

type tagsPage struct {
	Tags []string `json:"tags"`
}

func (c *Client) listTagsPage(
	ctx context.Context,
	pageURL *url.URL,
	token string,
) (tagsPage, string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	// #nosec G107 -- pageURL is constructed from a Docker Hub image name or validated against the registry origin.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL.String(), nil)
	if err != nil {
		return tagsPage{}, "", fmt.Errorf("create Docker Hub tags request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return tagsPage{}, "", fmt.Errorf("request Docker Hub tags: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode != http.StatusOK {
		return tagsPage{}, "", fmt.Errorf(
			"request Docker Hub tags: unexpected response status %s",
			response.Status,
		)
	}

	var page tagsPage
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		return tagsPage{}, "", fmt.Errorf("decode Docker Hub tags: %w", err)
	}

	return page, response.Header.Get("Link"), nil
}

func (c *Client) nextPageURL(link string) (*url.URL, error) {
	next, ok := nextLink(link)
	if !ok {
		return nil, nil
	}

	pageURL, err := url.Parse(next)
	if err != nil {
		return nil, fmt.Errorf("parse Docker Hub next page URL: %w", err)
	}
	pageURL = c.registryURL.ResolveReference(pageURL)
	if pageURL.Scheme != c.registryURL.Scheme || pageURL.Host != c.registryURL.Host {
		return nil, fmt.Errorf("unexpected origin in Docker Hub next page URL")
	}

	return pageURL, nil
}

func nextLink(link string) (string, bool) {
	for value := range strings.SplitSeq(link, ",") {
		urlPart, parameters, found := strings.Cut(value, ";")
		if !found || !strings.Contains(parameters, "rel=\"next\"") {
			continue
		}

		urlPart = strings.TrimSpace(urlPart)
		if strings.HasPrefix(urlPart, "<") && strings.HasSuffix(urlPart, ">") {
			return strings.TrimSuffix(strings.TrimPrefix(urlPart, "<"), ">"), true
		}
	}

	return "", false
}

func repositoryPath(imageName string) (string, error) {
	imageName = strings.TrimPrefix(imageName, "docker.io/")
	parts := strings.Split(imageName, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return "", fmt.Errorf("image name for Docker Hub is required")
		}

		return "library/" + parts[0], nil
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", fmt.Errorf("invalid Docker Hub image name %q", imageName)
		}

		return imageName, nil
	default:
		return "", fmt.Errorf("invalid Docker Hub image name %q", imageName)
	}
}
