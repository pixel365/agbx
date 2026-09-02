package prepare

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

type DockerClient interface {
	Build(context.Context, docker.BuildRequest) error
	Close() error
}

type DockerClientFunc func() (DockerClient, error)

func NewPrepareCommand(
	newDockerClient DockerClientFunc,
	providers *provider.Registry,
) *cobra.Command {
	return &cobra.Command{
		Use:   "prepare <provider>",
		Short: "Prepare a provider environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selectedProvider, err := providers.Lookup(args[0])
			if err != nil {
				if errors.Is(err, provider.ErrNotFound) {
					return fmt.Errorf("provider %q is not supported", args[0])
				}

				return err
			}

			configuration, err := commandconfig.Load(cmd)
			if err != nil {
				return err
			}
			recipe, err := selectedProvider.BuildRecipe(configuration.Image)
			if err != nil {
				return fmt.Errorf("create build recipe for provider %q: %w", args[0], err)
			}

			dockerClient, err := newDockerClient()
			if err != nil {
				return fmt.Errorf("create Docker client: %w", err)
			}
			defer func() {
				_ = dockerClient.Close()
			}()

			imageReference := recipe.PreparedImageReference(args[0], configuration.Image)
			if err := dockerClient.Build(cmd.Context(), docker.BuildRequest{
				Dockerfile: recipe.Dockerfile,
				BuildArgs:  recipe.BuildArgs,
				Output:     cmd.OutOrStdout(),
				Tag:        imageReference,
			}); err != nil {
				return fmt.Errorf("build provider %q: %w", args[0], err)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Prepared provider image: %s\n", imageReference)

			return err
		},
	}
}
