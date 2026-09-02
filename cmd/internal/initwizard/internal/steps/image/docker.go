package image

import (
	"context"

	"github.com/pixel365/agbx/internal/docker"
)

type dockerClient interface {
	ListImages(context.Context) ([]docker.Image, error)
	SearchImages(context.Context, string) ([]docker.SearchResult, error)
	Close() error
}

type newDockerClientFunc func() (dockerClient, error)

func newDockerClient() (dockerClient, error) {
	return docker.NewClient()
}
