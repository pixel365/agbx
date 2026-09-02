package initcommand

import (
	"errors"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/initwizard"
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
				confirmed, err := confirm(
					"Create a configuration file in the current directory?",
					"Create",
				)
				if err != nil || !confirmed {
					return err
				}

				configuration, err := initwizard.Default().Run(cmd.Context())
				if err != nil {
					return err
				}

				return config.Create(".", configuration)
			}
			if err != nil {
				return err
			}

			_, err = confirm("Replace the configuration file in the current directory?", "Replace")

			return err
		},
	}

	return cmd
}

func confirm(title string, affirmative string) (bool, error) {
	var confirmed bool
	err := huh.NewConfirm().
		Title(title).
		Affirmative(affirmative).
		Negative("Cancel").
		Value(&confirmed).
		Run()

	return confirmed, err
}
