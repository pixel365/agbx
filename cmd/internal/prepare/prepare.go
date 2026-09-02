package prepare

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/commandconfig"
	"github.com/pixel365/agbx/internal/provider"
)

func NewPrepareCommand(providers *provider.Registry) *cobra.Command {
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
			if _, err := selectedProvider.BuildRecipe(configuration.Image); err != nil {
				return fmt.Errorf("create build recipe for provider %q: %w", args[0], err)
			}

			return fmt.Errorf("build for provider %q is not implemented", args[0])
		},
	}
}
