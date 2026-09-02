package prepare

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/internal/provider"
)

func NewPrepareCommand(providers *provider.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "prepare <provider>",
		Short: "Prepare a provider environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if _, err := providers.Lookup(args[0]); err != nil {
				if errors.Is(err, provider.ErrNotFound) {
					return fmt.Errorf("provider %q is not supported", args[0])
				}

				return err
			}

			return fmt.Errorf("preparation for provider %q is not implemented", args[0])
		},
	}
}
