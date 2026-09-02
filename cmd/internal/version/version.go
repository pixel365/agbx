package version

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version     = "dev"
	commit      = "unknown"
	releaseDate = "unknown"
)

func NewVersionCommand() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !verbose {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), version)

				return err
			}

			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Version: %s\n", version); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(out, "Commit: %s\n", commit); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(out, "Release Date: %s\n", releaseDate); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(out, "Go Version: %s\n", runtime.Version()); err != nil {
				return err
			}
			_, err := fmt.Fprintf(out, "Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)

			return err
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Print version metadata")

	return cmd
}
