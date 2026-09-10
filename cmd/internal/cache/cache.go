package cache

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/preparedcache"
	"github.com/pixel365/agbx/internal/provider"
)

type DockerClient interface {
	Close() error
	ListPreparedImages(context.Context) ([]docker.PreparedImage, error)
}

type DockerClientFunc func() (DockerClient, error)

type listedImage struct {
	State string
	Image docker.PreparedImage
}

func NewCacheCommand(
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
) *cobra.Command {
	command := &cobra.Command{
		Use:   "cache",
		Short: "Inspect local agbx cache",
		Args:  cobra.NoArgs,
	}
	command.AddCommand(NewListCommand(newDockerClient, providers))

	return command
}

func NewListCommand(newDockerClient DockerClientFunc, providers *provider.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List prepared images for the current project",
		Args:  cobra.NoArgs,
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

func currentImageReferences(
	configuration config.Config,
	providers *provider.Registry,
) (map[string]string, error) {
	images := make(map[string]string)
	for _, selectedProvider := range providers.All() {
		recipe, err := provider.BuildRecipeFor(selectedProvider, configuration)
		if err != nil {
			return nil, fmt.Errorf(
				"create build recipe for provider %q: %w",
				selectedProvider.Name(),
				err,
			)
		}
		images[recipe.PreparedImageReference(selectedProvider.Name(), configuration.Image)] = selectedProvider.Name()
	}

	return images, nil
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

func writeImageTable(output io.Writer, images []listedImage) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "PROVIDER\tSTATE\tCREATED\tSIZE\tIMAGE"); err != nil {
		return err
	}
	for _, image := range images {
		if _, err := fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%s\t%s\n",
			image.Image.Provider,
			imageState(image),
			image.Image.CreatedAt.Format(time.DateOnly),
			formatSize(image.Image.Size),
			imageReference(image.Image),
		); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func imageState(image listedImage) string {
	state := image.State
	if !image.Image.Tagged {
		state += ", untagged"
	}
	if image.Image.Legacy {
		state += ", legacy"
	}

	return state
}

func imageReference(image docker.PreparedImage) string {
	if image.Tagged {
		return image.Reference
	}

	return image.Reference + " (" + image.ImageID + ")"
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	value := float64(size)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, name := range units {
		value /= unit
		if value < unit || name == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s", value, name)
		}
	}

	return fmt.Sprintf("%d B", size)
}
