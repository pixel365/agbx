package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/check"
	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/cmd/internal/initcommand"
	"github.com/pixel365/agbx/cmd/internal/prepare"
	"github.com/pixel365/agbx/cmd/internal/run"
	"github.com/pixel365/agbx/cmd/internal/version"
	"github.com/pixel365/agbx/internal/config"
	"github.com/pixel365/agbx/internal/docker"
	"github.com/pixel365/agbx/internal/provider"
)

type dockerClient interface {
	check.DockerClient
	prepare.DockerClient
	run.DockerClient
}

type dockerClientFunc func() (dockerClient, error)

func NewRootCommand() *cobra.Command {
	return newRootCommand(newDockerClient)
}

func newRootCommand(newDockerClient dockerClientFunc) *cobra.Command {
	var configFile string
	providers := provider.NewRegistry()

	cmd := &cobra.Command{
		Use: "agbx [command]",
	}

	cmd.PersistentFlags().StringVar(
		&configFile,
		"config",
		"",
		"Path to the configuration file",
	)
	cmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		switch cmd.Name() {
		case "init", "check", "run":
			return nil
		}

		_, err := commandconfig.Load(cmd)
		if errors.Is(err, config.ErrNotFound) {
			return nil
		}

		return err
	}

	cmd.AddCommand(
		initcommand.NewInitCommand(),
		prepare.NewPrepareCommand(func() (prepare.DockerClient, error) {
			return newDockerClient()
		}, providers),
		version.NewVersionCommand(),
		check.NewCheckCommand(func() (check.DockerClient, error) {
			return newDockerClient()
		}),
		run.NewRunCommand(func() (run.DockerClient, error) {
			return newDockerClient()
		}, providers),
	)

	return cmd
}

func newDockerClient() (dockerClient, error) {
	return docker.NewClient()
}
