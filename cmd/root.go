package cmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/version"
	"github.com/pixel365/agbx/internal/config"
)

func NewRootCommand(ctx context.Context) *cobra.Command {
	var configFile string

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
		if cmd.Root().PersistentFlags().Changed("config") {
			_, err := config.Load(configFile)

			return err
		}

		_, err := config.LoadDefault(".")
		if errors.Is(err, config.ErrNotFound) {
			return nil
		}

		return err
	}

	cmd.AddCommand(
		version.NewVersionCommand(),
	)

	return cmd
}
