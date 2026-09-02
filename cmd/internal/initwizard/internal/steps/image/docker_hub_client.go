package image

import (
	"context"

	"github.com/pixel365/agbx/internal/dockerhub"
)

type dockerHubClient interface {
	ListTags(context.Context, string) ([]string, error)
	ResolveDigest(context.Context, string, string) (string, error)
}

type newDockerHubClientFunc func() dockerHubClient

func newDockerHubClient() dockerHubClient {
	return dockerhub.NewClient()
}
