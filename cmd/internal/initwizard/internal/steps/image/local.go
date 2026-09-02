package image

import (
	"context"
	"fmt"
	"strings"

	"charm.land/huh/v2"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
)

const (
	localSourceKey       = "local"
	localImageListHeight = 10
)

type localSource struct {
	newClient newDockerClientFunc
}

func (localSource) key() string {
	return localSourceKey
}

func (localSource) label() string {
	return "Choose from local Docker images"
}

func (source localSource) Run(ctx context.Context, configuration *config.Config) error {
	client, err := source.newClient()
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer func() {
		_ = client.Close()
	}()

	var images []docker.Image
	if err := runWithSpinner(
		ctx,
		"Loading local Docker images...",
		func(ctx context.Context) error {
			var err error
			images, err = client.ListImages(ctx)

			return err
		},
	); err != nil {
		return fmt.Errorf("list local Docker images: %w", err)
	}
	if len(images) == 0 {
		return fmt.Errorf("no tagged local Docker images found")
	}

	var filter string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Filter local Docker images").
				Description("Optional; leave empty to show all images.").
				Value(&filter),
		),
	).Run(); err != nil {
		return err
	}
	images = filterImages(images, filter)
	if len(images) == 0 {
		return fmt.Errorf("no local Docker images match %q", filter)
	}

	selectedImage := ""
	options := make([]huh.Option[string], 0, len(images))
	for _, image := range images {
		reference := image.Name + ":" + image.Tag
		options = append(options, huh.NewOption(reference, reference))
	}
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a local Docker image").
				Options(options...).
				Filtering(true).
				Height(localImageListHeight).
				Value(&selectedImage),
		),
	).Run(); err != nil {
		return err
	}

	for _, image := range images {
		if image.Name+":"+image.Tag == selectedImage {
			configuration.Image.Name = image.Name
			configuration.Image.Tag = image.Tag
			configuration.Image.Digest = image.Digest

			return nil
		}
	}

	return fmt.Errorf("unknown local Docker image %q", selectedImage)
}

func filterImages(images []docker.Image, filter string) []docker.Image {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return images
	}

	filteredImages := make([]docker.Image, 0, len(images))
	for _, image := range images {
		reference := image.Name + ":" + image.Tag
		if strings.Contains(strings.ToLower(reference), filter) {
			filteredImages = append(filteredImages, image)
		}
	}

	return filteredImages
}

var _ source = localSource{}
