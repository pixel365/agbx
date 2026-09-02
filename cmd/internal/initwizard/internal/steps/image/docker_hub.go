package image

import (
	"context"
	"fmt"

	"charm.land/huh/v2"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
)

const (
	dockerHubSourceKey = "docker-hub"
	latestTagChoice    = "latest"
	manualTagChoice    = "manual"
	browseTagsChoice   = "browse"
)

type dockerHubSource struct {
	newDockerClient newDockerClientFunc
	newHubClient    newDockerHubClientFunc
}

func (dockerHubSource) key() string {
	return dockerHubSourceKey
}

func (dockerHubSource) label() string {
	return "Search Docker Hub"
}

func (source dockerHubSource) Run(ctx context.Context, configuration *config.Config) error {
	var query string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Search Docker Hub").
				Value(&query).
				Validate(requiredValue("Docker Hub search query")),
		),
	).Run(); err != nil {
		return err
	}

	dockerClient, err := source.newDockerClient()
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer func() {
		_ = dockerClient.Close()
	}()

	var results []docker.SearchResult
	if err := runWithSpinner(
		ctx,
		fmt.Sprintf("Searching Docker Hub for %q...", query),
		func(ctx context.Context) error {
			var err error
			results, err = dockerClient.SearchImages(ctx, query)

			return err
		},
	); err != nil {
		return fmt.Errorf("search Docker Hub images: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("no Docker Hub images found for %q", query)
	}

	selectedImage := ""
	options := make([]huh.Option[string], 0, len(results))
	for _, result := range results {
		options = append(options, huh.NewOption(result.Name, result.Name))
	}
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a Docker Hub image").
				Options(options...).
				Filtering(true).
				Value(&selectedImage),
		),
	).Run(); err != nil {
		return err
	}

	configuration.Image.Name = selectedImage
	configuration.Image.Digest = ""

	var tagChoice string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(dockerImageTag).
				Options(
					huh.NewOption("Use latest", latestTagChoice),
					huh.NewOption("Enter a tag manually", manualTagChoice),
					huh.NewOption("Browse all tags (may be slow)", browseTagsChoice),
				).
				Value(&tagChoice),
		),
	).Run(); err != nil {
		return err
	}

	switch tagChoice {
	case latestTagChoice:
		configuration.Image.Tag = latestTagChoice

		return nil
	case manualTagChoice:
		if err := promptDockerHubTag(configuration); err != nil {
			return err
		}
	case browseTagsChoice:
		if err := source.chooseTagFromDockerHub(ctx, configuration); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown Docker Hub tag choice %q", tagChoice)
	}

	return source.resolveDigest(ctx, configuration)
}

func promptDockerHubTag(configuration *config.Config) error {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(dockerImageTag).
				Value(&configuration.Image.Tag).
				Validate(requiredValue(dockerImageTag)),
		),
	).Run()
}

func (source dockerHubSource) chooseTagFromDockerHub(
	ctx context.Context,
	configuration *config.Config,
) error {
	hubClient := source.newHubClient()
	var tags []string
	if err := runWithSpinner(
		ctx,
		fmt.Sprintf("Loading Docker Hub tags for %q...", configuration.Image.Name),
		func(ctx context.Context) error {
			var err error
			tags, err = hubClient.ListTags(ctx, configuration.Image.Name)

			return err
		},
	); err != nil {
		return fmt.Errorf("list Docker Hub tags for %q: %w", configuration.Image.Name, err)
	}
	if len(tags) == 0 {
		return fmt.Errorf("no Docker Hub tags found for %q", configuration.Image.Name)
	}

	selectedTag := ""
	options := make([]huh.Option[string], 0, len(tags))
	for _, tag := range tags {
		options = append(options, huh.NewOption(tag, tag))
	}
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a Docker Hub image tag").
				Options(options...).
				Filtering(true).
				Height(localImageListHeight).
				Value(&selectedTag),
		),
	).Run(); err != nil {
		return err
	}

	configuration.Image.Tag = selectedTag

	return nil
}

func (source dockerHubSource) resolveDigest(
	ctx context.Context,
	configuration *config.Config,
) error {
	if configuration.Image.Tag == latestTagChoice {
		configuration.Image.Digest = ""

		return nil
	}

	hubClient := source.newHubClient()
	var digest string
	if err := runWithSpinner(
		ctx,
		"Resolving Docker Hub digest...",
		func(ctx context.Context) error {
			var err error
			digest, err = hubClient.ResolveDigest(
				ctx,
				configuration.Image.Name,
				configuration.Image.Tag,
			)

			return err
		},
	); err != nil {
		return fmt.Errorf(
			"resolve Docker Hub digest for %q:%s: %w",
			configuration.Image.Name,
			configuration.Image.Tag,
			err,
		)
	}

	configuration.Image.Digest = digest

	return nil
}

var _ source = dockerHubSource{}
