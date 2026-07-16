package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// TranslationOptions holds the flag values shared by the root command and
// the explicit translate command.
type TranslationOptions struct {
	Text    string
	Context string
	Profile string
	Tone    string
}

// addTranslationFlags registers the translation flags shared by the root
// and translate commands. They are local (not persistent) flags: setup and
// version do not expose them.
func addTranslationFlags(cmd *cobra.Command, opts *TranslationOptions) {
	cmd.Flags().StringVar(&opts.Text, "text", "", "text to translate; omit to read stdin or launch the interactive UI")
	cmd.Flags().StringVar(&opts.Context, "context", "", "optional reference context used only to disambiguate the translation")
	cmd.Flags().StringVar(&opts.Profile, "profile", "", "profile to use for this translation (defaults to the active profile)")
	cmd.Flags().StringVar(&opts.Tone, "tone", prompt.DefaultTone.String(), "translation tone: neutral, literal, or diplomatic")
}

func newTranslateCommand(deps app.Dependencies) *cobra.Command {
	opts := TranslationOptions{}
	cmd := &cobra.Command{
		Use:           "translate",
		Short:         "Translate Russian text into English (same as the root command)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTranslation(cmd, opts, deps)
		},
	}
	addTranslationFlags(cmd, &opts)
	return cmd
}

// runTranslation is the shared handler behind the root command and the
// explicit translate command. It selects a mode in the exact order the
// specification requires:
//
//   - A: --text was explicitly supplied -> one non-interactive translation.
//   - B: --text omitted, stdin is not a terminal -> read stdin, translate.
//   - C: --text omitted, stdin and stdout are both terminals -> launch the TUI.
//   - D: stdin is a terminal but stdout is not -> error.
func runTranslation(cmd *cobra.Command, opts TranslationOptions, deps app.Dependencies) error {
	quiet, _ := cmd.Flags().GetBool("quiet")

	tone, err := prompt.ParseTone(opts.Tone)
	if err != nil {
		return fail(deps, err)
	}

	switch {
	case cmd.Flags().Changed("text"):
		return runBatchTranslation(cmd, opts, opts.Text, tone, quiet, deps)
	case !deps.StdinIsTerminal():
		data, err := io.ReadAll(deps.Stdin)
		if err != nil {
			return fail(deps, fmt.Errorf("read stdin: %w", err))
		}
		return runBatchTranslation(cmd, opts, string(data), tone, quiet, deps)
	case deps.StdoutIsTerminal():
		return runInteractive(opts, tone, deps)
	default:
		return fail(deps, errors.New("interactive mode requires terminal output; use --text or pipe source text to stdin"))
	}
}

func runBatchTranslation(cmd *cobra.Command, opts TranslationOptions, text string, tone prompt.Tone, quiet bool, deps app.Dependencies) error {
	if strings.TrimSpace(text) == "" {
		return fail(deps, errors.New("source text must not be empty"))
	}

	cfg, _, err := config.Effective(deps.ConfigPath, deps.Getenv, opts.Profile)
	if err != nil {
		return fail(deps, fmt.Errorf("configuration error: %w (run \"qt setup\")", err))
	}

	translator, err := deps.NewTranslator(cfg)
	if err != nil {
		return fail(deps, err)
	}

	reporter := translate.BatchReporter{Writer: deps.Stderr, Quiet: quiet}
	input := translate.TranslationInput{Source: text, Context: opts.Context, Tone: tone}
	result, err := translator.Translate(cmd.Context(), input, reporter)
	if err != nil {
		// The reporter has already written a safe failure line to stderr.
		return newRunError(err, 1)
	}

	fmt.Fprintln(deps.Stdout, result)
	return nil
}

func runInteractive(opts TranslationOptions, tone prompt.Tone, deps app.Dependencies) error {
	// Validate up front so an unconfigured or bad profile fails with a clear
	// message instead of launching an empty UI.
	if _, _, err := config.Effective(deps.ConfigPath, deps.Getenv, opts.Profile); err != nil {
		return fail(deps, fmt.Errorf("configuration error: %w (run \"qt setup\")", err))
	}

	err := deps.RunTUI(deps.Stdin, deps.Stdout, deps.ConfigPath, deps.Getenv, deps.NewTranslator, opts.Profile, tone)
	if err != nil {
		if errors.Is(err, app.ErrInterrupted) {
			return err
		}
		return fail(deps, err)
	}
	return nil
}
