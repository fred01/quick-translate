// Package app wires the operating-system resources and production behavior
// that the CLI depends on, so that only cmd/qt/main.go touches os.Args,
// os.Stdin, os.Stdout, os.Stderr, and os.Exit.
package app

import (
	"errors"
	"io"

	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/translate"
)

// ErrInterrupted signals that the user quit via Ctrl+C. It maps to exit code
// 130.
var ErrInterrupted = errors.New("interrupted")

// TUIRunner launches the interactive translation UI. It loads the profile
// store from configPath itself and can manage profiles (add, edit, delete,
// switch active) and re-test/save them in place, which is why it takes the
// path, the environment lookup, the translator factory, and an optional
// per-run profile override rather than a pre-built Translator. It returns
// ErrInterrupted when the user quits, since Ctrl+C is the TUI's only way to
// exit.
type TUIRunner func(
	in io.Reader,
	out io.Writer,
	configPath string,
	getenv config.EnvLookup,
	newTranslator func(config.Config) (translate.Translator, error),
	profileOverride string,
) error

// SetupResult summarizes a completed interactive setup for display.
type SetupResult struct {
	Profile     string
	Config      config.Config
	Translation string
}

// SetupFormRunner runs the complete interactive qt setup experience: a form
// to name and configure a profile (prefilled from the store), a spinner
// while testing the candidate through newTranslator's production translation
// path, and — only on success — upserting the profile into the store at path
// and marking it active. The existing API key of a profile is never
// displayed; a blank key input keeps the existing key when editing a profile
// that already has one.
type SetupFormRunner func(
	in io.Reader,
	out io.Writer,
	configPath string,
	store config.Store,
	newTranslator func(config.Config) (translate.Translator, error),
) (SetupResult, error)

// Dependencies collects everything the CLI needs from its environment.
// Production wiring lives in cmd/qt/main.go; tests supply fakes so that CLI
// behavior can be exercised without a real terminal or network access.
type Dependencies struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	ConfigPath string
	Getenv     config.EnvLookup

	StdinIsTerminal  func() bool
	StdoutIsTerminal func() bool

	// NewTranslator builds the production Translator for an effective
	// configuration. Defaults to translate.NewTranslator.
	NewTranslator func(config.Config) (translate.Translator, error)

	RunTUI       TUIRunner
	RunSetupForm SetupFormRunner
}
