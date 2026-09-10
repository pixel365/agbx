package cache

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/preparedcache"
	"github.com/pixel365/agbx/internal/provider"
)

func NewListCommand(newDockerClient DockerClientFunc, providers *provider.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List prepared images for the current project",
		Long: "List prepared images associated with the selected project. Current images match " +
			"the active configuration; historical images were previously selected for the project. " +
			"Legacy images were created before image labels were introduced, and unattributed images " +
			"belong to another or an unknown project.",
		Example: "  agbx cache list\n" +
			"  agbx --config /path/to/.agbx.yaml cache list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listImages(cmd, newDockerClient, providers)
		},
	}
}

func listImages(
	cmd *cobra.Command,
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
) error {
	loadedConfig, err := commandconfig.LoadWithPath(cmd)
	if err != nil {
		return err
	}
	currentImages, err := currentImageReferences(loadedConfig.Configuration, providers)
	if err != nil {
		return err
	}
	index, err := preparedcache.Load(loadedConfig.FilePath)
	if err != nil {
		return fmt.Errorf("load prepared image index: %w", err)
	}

	dockerClient, err := newDockerClient()
	if err != nil {
		return fmt.Errorf("create Docker client: %w", err)
	}
	defer func() {
		_ = dockerClient.Close()
	}()
	images, err := dockerClient.ListPreparedImages(cmd.Context())
	if err != nil {
		return fmt.Errorf("list prepared images: %w", err)
	}

	projectImages, unattributedImages := classifyImages(images, currentImages, index)
	return writeImages(cmd.OutOrStdout(), loadedConfig.FilePath, projectImages, unattributedImages)
}

func writeImages(
	output io.Writer,
	configFile string,
	projectImages []listedImage,
	unattributedImages []listedImage,
) error {
	if _, err := fmt.Fprintf(output, "Project: %s\n", configFile); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Prepared images: %d\n", len(projectImages)); err != nil {
		return err
	}
	if len(projectImages) == 0 {
		if _, err := fmt.Fprintln(
			output,
			"No prepared images are associated with this project.",
		); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(output); err != nil {
			return err
		}
		if err := writeImageTable(output, projectImages); err != nil {
			return err
		}
	}
	if len(unattributedImages) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(output, "\nUnattributed prepared images:"); err != nil {
		return err
	}

	return writeImageTable(output, unattributedImages)
}

func classifyImages(
	images []docker.PreparedImage,
	currentImages map[string]string,
	index preparedcache.Project,
) ([]listedImage, []listedImage) {
	projectImages := make([]listedImage, 0)
	unattributedImages := make([]listedImage, 0)
	for _, image := range images {
		if currentProvider, ok := currentImages[image.Reference]; ok &&
			currentProvider == image.Provider && image.Tagged {
			projectImages = append(projectImages, listedImage{Image: image, State: "current"})

			continue
		}
		if isHistoricalImage(index, image) {
			projectImages = append(projectImages, listedImage{Image: image, State: "historical"})

			continue
		}
		unattributedImages = append(
			unattributedImages,
			listedImage{Image: image, State: "unattributed"},
		)
	}

	return projectImages, unattributedImages
}

func isHistoricalImage(index preparedcache.Project, image docker.PreparedImage) bool {
	provider, ok := index.Providers[image.Provider]
	if !ok {
		return false
	}
	_, ok = provider.Images[image.Reference]

	return ok
}
