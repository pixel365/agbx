package prepare

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewPrepareCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "prepare <provider>",
		Short: "Prepare a provider environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return fmt.Errorf("provider %q is not supported", args[0])
		},
	}
}
