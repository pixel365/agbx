package cache

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

type DockerClient interface {
	Close() error
	ListPreparedImages(context.Context) ([]docker.PreparedImage, error)
	RemovePreparedImage(context.Context, docker.PreparedImage) error
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
		Long: "Inspect prepared provider images stored by Docker. Use cache list to see the " +
			"images associated with the selected project and other AGBX images on the host.",
		Example: "  agbx cache list",
		Args:    cobra.NoArgs,
	}
	command.AddCommand(
		NewListCommand(newDockerClient, providers),
		NewPruneCommand(newDockerClient, providers),
	)

	return command
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
