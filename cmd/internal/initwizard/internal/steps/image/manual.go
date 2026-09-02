package image

import (
	"context"
	"errors"
	"strings"

	"charm.land/huh/v2"

	"github.com/pixel365/agbx/internal/config"
)

const manualSourceKey = "manual"

type manualSource struct{}

func (manualSource) key() string {
	return manualSourceKey
}

func (manualSource) label() string {
	return "Enter an image reference manually"
}

func (manualSource) Run(_ context.Context, configuration *config.Config) error {
	return promptImageReference(configuration)
}

func promptImageReference(configuration *config.Config) error {
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Docker image name").
				Value(&configuration.Image.Name).
				Validate(requiredValue("Docker image name")),
			huh.NewInput().
				Title(dockerImageTag).
				Value(&configuration.Image.Tag).
				Validate(requiredValue(dockerImageTag)),
		),
	).Run(); err != nil {
		return err
	}

	if configuration.Image.Tag == "latest" {
		configuration.Image.Digest = ""

		return nil
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Docker image digest").
				Description("Optional; pin the image with a sha256 digest.").
				Value(&configuration.Image.Digest),
		),
	).Run()
}

func requiredValue(name string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New(name + " is required")
		}

		return nil
	}
}
