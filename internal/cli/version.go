package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/version"
)

func newVersionCommand(deps app.Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:           "version",
		Short:         "Print the qt version",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(deps.Stdout, "qt %s\n", version.Version)
			return nil
		},
	}
}
