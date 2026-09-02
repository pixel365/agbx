package image

import (
	"context"
	"fmt"

	"charm.land/huh/v2"

	"github.com/pixel365/agbx/internal/config"
)

const dockerImageTag = "Docker image tag"

type source interface {
	key() string
	label() string
	Run(context.Context, *config.Config) error
}

type Step struct {
	sources []source
}

func New() Step {
	newDockerClient := newDockerClient

	return newStep(
		localSource{newClient: newDockerClient},
		dockerHubSource{
			newDockerClient: newDockerClient,
			newHubClient:    newDockerHubClient,
		},
		manualSource{},
	)
}

func newStep(sources ...source) Step {
	return Step{sources: sources}
}

func (step Step) Run(ctx context.Context, configuration *config.Config) error {
	var selectedSource string
	options := make([]huh.Option[string], 0, len(step.sources))
	for _, source := range step.sources {
		options = append(options, huh.NewOption(source.label(), source.key()))
	}

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Docker image source").
				Options(options...).
				Value(&selectedSource),
		),
	).Run()
	if err != nil {
		return err
	}

	for _, source := range step.sources {
		if source.key() == selectedSource {
			return source.Run(ctx, configuration)
		}
	}

	return fmt.Errorf("unknown Docker image source %q", selectedSource)
}
