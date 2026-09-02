package initcommand

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/internal/config"
)

func NewInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "init",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Root().PersistentFlags().Changed("config") {
				return errors.New("--config cannot be used with init")
			}

			_, err := config.LoadDefault(".")
			if errors.Is(err, config.ErrNotFound) {
				_, err = fmt.Fprintln(
					cmd.OutOrStdout(),
					"No configuration file found in the current directory. Do you want to create it?",
				)
			} else if err == nil {
				_, err = fmt.Fprintln(
					cmd.OutOrStdout(),
					"Configuration file found in the current directory. Do you want to replace it?",
				)
			}

			return err
		},
	}

	return cmd
}
