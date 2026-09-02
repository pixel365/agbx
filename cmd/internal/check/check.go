package check

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/internal/config"
)

func NewCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate the configuration file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Root().PersistentFlags().Changed("config") {
				configFile, err := cmd.Root().PersistentFlags().GetString("config")
				if err != nil {
					return fmt.Errorf("get config flag: %w", err)
				}

				if _, err := config.Load(configFile); err != nil {
					return err
				}
			} else if _, err := config.LoadDefault("."); err != nil {
				return err
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), "Configuration is valid.")

			return err
		},
	}
}
