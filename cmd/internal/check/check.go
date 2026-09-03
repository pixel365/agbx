package check

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/provider"
)

const dockerPingTimeout = 5 * time.Second

type DockerClient interface {
	Close() error
	HasImage(context.Context, string) (bool, error)
	Ping(context.Context) error
}

type DockerClientFunc func() (DockerClient, error)

func NewCheckCommand(
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
) *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate the configuration and Docker daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configuration, err := commandconfig.Load(cmd)
			if err != nil {
				return err
			}

			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Configuration is valid."); err != nil {
				return err
			}

			dockerClient, err := newDockerClient()
			if err != nil {
				return fmt.Errorf("create Docker client: %w", err)
			}
			defer func() {
				_ = dockerClient.Close()
			}()

			ctx, cancel := context.WithTimeout(cmd.Context(), dockerPingTimeout)
			defer cancel()
			if err := dockerClient.Ping(ctx); err != nil {
				return fmt.Errorf("ping Docker daemon: %w", err)
			}

			if _, err := fmt.Fprintln(
				cmd.OutOrStdout(),
				"Docker daemon is available.",
			); err != nil {
				return err
			}
			if !verbose {
				return nil
			}

			return writeProviderStatuses(cmd, ctx, dockerClient, providers, configuration.Image)
		},
	}
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show provider image status")

	return cmd
}

func writeProviderStatuses(
	cmd *cobra.Command,
	ctx context.Context,
	dockerClient DockerClient,
	providers *provider.Registry,
	image config.Image,
) error {
	registeredProviders := providers.All()
	if len(registeredProviders) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
		return err
	}

	for index, selectedProvider := range registeredProviders {
		recipe, err := selectedProvider.BuildRecipe(image)
		if err != nil {
			return fmt.Errorf(
				"create build recipe for provider %q: %w",
				selectedProvider.Name(),
				err,
			)
		}
		imageReference := recipe.PreparedImageReference(selectedProvider.Name(), image)
		hasImage, err := dockerClient.HasImage(ctx, imageReference)
		if err != nil {
			return fmt.Errorf("check prepared image %q: %w", imageReference, err)
		}

		status := "not prepared"
		if hasImage {
			status = "prepared"
		}

		if index > 0 {
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"Provider: %s\nImage: %s\nStatus: %s\n",
			selectedProvider.Name(),
			imageReference,
			status,
		); err != nil {
			return err
		}
	}

	return nil
}
