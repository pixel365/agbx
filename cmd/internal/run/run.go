package run

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/docker"
)

type DockerClient interface {
	Run(context.Context, docker.RunRequest) error
	Close() error
}

type DockerClientFunc func() (DockerClient, error)

func NewRunCommand(newDockerClient DockerClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "run <command> [arguments...]",
		Short: "Run a command in the configured container",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configuration, err := commandconfig.Load(cmd)
			if err != nil {
				return err
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
				Command:          args,
				Image:            configuration.Image.Reference(),
				Input:            cmd.InOrStdin(),
				Output:           cmd.OutOrStdout(),
				WorkingDirectory: workingDirectory,
			})
		},
	}
}
