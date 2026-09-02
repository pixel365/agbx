package check

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
)

const dockerPingTimeout = 5 * time.Second

type DockerClient interface {
	Ping(context.Context) error
	Close() error
}

type DockerClientFunc func() (DockerClient, error)

func NewCheckCommand(newDockerClient DockerClientFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate the configuration and Docker daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := commandconfig.Load(cmd); err != nil {
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

			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Docker daemon is available.")

			return err
		},
	}
}
