package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/pixel365/agbx/cmd/internal/version"
)

func NewRootCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use: "agbx [command]",
	}

	cmd.AddCommand(
		version.NewVersionCommand(),
	)

	return cmd
}
