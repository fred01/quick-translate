// Package cli builds the qt Cobra command tree. Commands are constructed
// through functions taking an explicit Dependencies value; there are no
// package-level mutable Cobra commands or flag variables.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/fred01/quick-translate/internal/app"
)

// RunError tags an error as already having been reported to the user (its
// message written to stderr by the command that produced it) and carries
// the process exit code it should map to.
type RunError struct {
	Err  error
	Code int
}

func (e *RunError) Error() string { return e.Err.Error() }
func (e *RunError) Unwrap() error { return e.Err }

func newRunError(err error, code int) error {
	if err == nil {
		return nil
	}
	return &RunError{Err: err, Code: code}
}

// fail prints a concise, safe error message to stderr and wraps err as an
// application failure (exit code 1).
func fail(deps app.Dependencies, err error) error {
	fmt.Fprintf(deps.Stderr, "qt: %s\n", err)
	return newRunError(err, 1)
}

// ExitCode maps an error returned from the root command's Execute to a
// process exit code: 0 for success, 130 for a user interruption, 1 for a
// tagged application failure, and 2 for anything else (a Cobra-level usage
// error such as an unknown command or flag).
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, app.ErrInterrupted) {
		return 130
	}
	var re *RunError
	if errors.As(err, &re) {
		return re.Code
	}
	return 2
}

// Execute runs root and returns the process exit code. Errors not already
// tagged with RunError originate from Cobra's own argument/flag parsing and
// have not been printed anywhere yet, so Execute prints them here.
func Execute(root *cobra.Command, deps app.Dependencies) int {
	err := root.Execute()
	if err != nil {
		var re *RunError
		if !errors.As(err, &re) {
			fmt.Fprintf(deps.Stderr, "qt: %s\n", err)
		}
	}
	return ExitCode(err)
}

// NewRootCommand builds the qt root command and its subcommands.
func NewRootCommand(deps app.Dependencies) *cobra.Command {
	opts := TranslationOptions{}

	root := &cobra.Command{
		Use:           "qt",
		Short:         "Translate Russian text into clear, natural, polished business English",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTranslation(cmd, opts, deps)
		},
	}
	root.SetIn(deps.Stdin)
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)

	root.PersistentFlags().Bool("quiet", false, "suppress successful operational status in batch mode")
	addTranslationFlags(root, &opts)

	root.AddCommand(newTranslateCommand(deps))
	root.AddCommand(newScriptCommand(deps))
	root.AddCommand(newSetupCommand(deps))
	root.AddCommand(newVersionCommand(deps))

	return root
}
