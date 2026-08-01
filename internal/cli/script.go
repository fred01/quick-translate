package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/script"
	"github.com/fred01/quick-translate/internal/translate"
)

// ScriptOptions holds the flags of the script command.
type ScriptOptions struct {
	Output    string
	Context   string
	Profile   string
	Tone      string
	ChunkSize int
}

const scriptLong = `Translate a narration script, leaving its timing alone.

A narration script is the format speech-translator's ` + "`stt`" + ` writes and its
` + "`tts`" + ` reads: lines of speech, placed on a soundtrack by timestamps and
delays.

    @voice am_eric
    [0:02] Привет! Сегодня я покажу новые возможности Orca C L I.
    $delay(0.5s) Первая из них - пресеты.
    [1:20] Дальше - снапшоты. Запустим и подождём.

Only the spoken words are sent anywhere. Timestamps, ` + "`$delay`" + `, ` + "`@`" + `
directives, comments, and blank lines are carried through byte for byte - not
because the model is asked to preserve them, but because it never sees them.

The whole script goes in one request, so the model sees the narration as one
piece: a term introduced in the opening decides how it is rendered forty lines
later. ` + "`--chunk`" + ` splits it for a script that will not fit, and then every
chunk carries the lines around it as context.

    qt script demo.script -o demo.en.script
    qt script demo.script --tone literal
    cat demo.script | qt script > demo.en.script`

func newScriptCommand(deps app.Dependencies) *cobra.Command {
	opts := ScriptOptions{}
	cmd := &cobra.Command{
		Use:           "script [file]",
		Short:         "Translate a narration script, preserving its timestamps and delays",
		Long:          scriptLong,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScript(cmd, args, opts, deps)
		},
	}
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "write the translated script here (default: stdout)")
	cmd.Flags().StringVar(&opts.Context, "context", "", "optional reference context used only to disambiguate the translation")
	cmd.Flags().StringVar(&opts.Profile, "profile", "", "profile to use for this translation (defaults to the active profile)")
	cmd.Flags().StringVar(&opts.Tone, "tone", prompt.DefaultTone.String(), "translation tone: neutral, literal, or diplomatic")
	cmd.Flags().IntVar(&opts.ChunkSize, "chunk", 0,
		"split into requests of this many spoken lines (default: one request for the whole script)")
	return cmd
}

func runScript(cmd *cobra.Command, args []string, opts ScriptOptions, deps app.Dependencies) error {
	quiet, _ := cmd.Flags().GetBool("quiet")

	tone, err := prompt.ParseTone(opts.Tone)
	if err != nil {
		return fail(deps, err)
	}

	source, err := readScript(args, deps)
	if err != nil {
		return fail(deps, err)
	}
	parsed := script.Parse(source)
	if parsed.SpokenCount() == 0 {
		return fail(deps, errors.New("no spoken lines found: every line is a comment, a directive, or timing"))
	}

	cfg, _, err := config.Effective(deps.ConfigPath, deps.Getenv, opts.Profile)
	if err != nil {
		return fail(deps, fmt.Errorf("configuration error: %w (run \"qt setup\")", err))
	}
	translator, err := deps.NewTranslator(cfg)
	if err != nil {
		return fail(deps, err)
	}

	if !quiet {
		fmt.Fprintf(deps.Stderr, "qt: %d spoken line(s) of %d, %d untouched\n",
			parsed.SpokenCount(), len(parsed.Lines), len(parsed.Lines)-parsed.SpokenCount())
	}

	reporter := translate.BatchReporter{Writer: deps.Stderr, Quiet: quiet}
	result, err := script.Translate(cmd.Context(), translator, parsed,
		script.Options{Tone: tone, Context: opts.Context, ChunkSize: opts.ChunkSize},
		reporter, scriptProgress{writer: deps.Stderr, quiet: quiet})
	if err != nil {
		// The reporter has already written a safe failure line to stderr.
		return newRunError(err, 1)
	}

	rendered := parsed.RenderInto(result.Translations)
	if err := writeScript(opts.Output, rendered, deps); err != nil {
		return fail(deps, err)
	}

	// Untranslated lines are left in Russian on purpose, so this warning is the
	// only thing standing between the user and a script that looks finished.
	if len(result.Untranslated) > 0 {
		fmt.Fprintf(deps.Stderr, "qt: %d line(s) came back untranslated and were left in the source language: %s\n",
			len(result.Untranslated), joinNumbers(result.Untranslated))
		return newRunError(errors.New("some lines were not translated"), 1)
	}
	return nil
}

func readScript(args []string, deps app.Dependencies) (string, error) {
	if len(args) == 1 && args[0] != "-" {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", args[0], err)
		}
		return string(data), nil
	}
	if len(args) == 0 && deps.StdinIsTerminal() {
		return "", errors.New("no script: give a file, or pipe one to stdin")
	}
	data, err := io.ReadAll(deps.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return string(data), nil
}

func writeScript(output, rendered string, deps app.Dependencies) error {
	if output == "" || output == "-" {
		fmt.Fprint(deps.Stdout, rendered)
		return nil
	}
	if err := os.WriteFile(output, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", output, err)
	}
	fmt.Fprintln(deps.Stdout, output)
	return nil
}

func joinNumbers(numbers []int) string {
	parts := make([]string, 0, len(numbers))
	for _, number := range numbers {
		parts = append(parts, fmt.Sprint(number))
	}
	return strings.Join(parts, ", ")
}

// scriptProgress reports chunk completion, which for a long script is the only
// sign of life between the first request and the last.
type scriptProgress struct {
	writer io.Writer
	quiet  bool
}

func (p scriptProgress) Chunk(done, total int) {
	if p.quiet || total < 2 {
		return
	}
	fmt.Fprintf(p.writer, "qt: batch %d/%d done\n", done, total)
}
