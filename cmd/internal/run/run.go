package run

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

type DockerClient interface {
	Run(context.Context, docker.RunRequest) error
	Close() error
}

type DockerClientFunc func() (DockerClient, error)

func NewRunCommand(newDockerClient DockerClientFunc, providers *provider.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "run <provider> [arguments...]",
		Short: "Run a provider in the configured container",
		Args:  cobra.MinimumNArgs(1),
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
			command, err := selectedProvider.Command(args[1:])
			if err != nil {
				return fmt.Errorf("create command for provider %q: %w", args[0], err)
			}

			workingDirectory, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}

			dockerClient, err := newDockerClient()
			if err != nil {
				return fmt.Errorf("create Docker client: %w", err)
			}
			defer func() {
				_ = dockerClient.Close()
			}()

			return dockerClient.Run(cmd.Context(), docker.RunRequest{
				Command: command,
				Image: recipe.PreparedImageReference(
					selectedProvider.Name(),
					configuration.Image,
				),
				Input:            cmd.InOrStdin(),
				Output:           cmd.OutOrStdout(),
				PullImage:        false,
				WorkingDirectory: workingDirectory,
			})
		},
	}
}
