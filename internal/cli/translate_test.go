package cli

import (
	"strings"
	"testing"

	"github.com/fred01/quick-translate/internal/config"
)

func TestRootTextFlag(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--text", "hello"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if got := h.Stdout.String(); got != "translated\n" {
		t.Fatalf("stdout = %q, want %q", got, "translated\n")
	}
	if h.Translator.calls != 1 {
		t.Fatalf("Translate called %d times, want 1", h.Translator.calls)
	}
	if h.Translator.lastInput.Source != "hello" {
		t.Fatalf("Source = %q, want %q", h.Translator.lastInput.Source, "hello")
	}
}

func TestTranslateSubcommandTextFlag(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"translate", "--text", "hello"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if got := h.Stdout.String(); got != "translated\n" {
		t.Fatalf("stdout = %q, want %q", got, "translated\n")
	}
	if h.Translator.calls != 1 {
		t.Fatalf("Translate called %d times, want 1", h.Translator.calls)
	}
}

func TestRootAndTranslateShareTheSameHandler(t *testing.T) {
	// Both invocations should produce identical stdout/behavior for the same
	// input, since they are wired to the same runTranslation function.
	h1 := newHarness(t)
	root1 := NewRootCommand(h1.Deps())
	root1.SetArgs([]string{"--text", "hello", "--context", "ctx"})
	if err := root1.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	h2 := newHarness(t)
	root2 := NewRootCommand(h2.Deps())
	root2.SetArgs([]string{"translate", "--text", "hello", "--context", "ctx"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if h1.Stdout.String() != h2.Stdout.String() {
		t.Fatalf("root stdout %q != translate stdout %q", h1.Stdout.String(), h2.Stdout.String())
	}
	if h1.Translator.lastInput != h2.Translator.lastInput {
		t.Fatalf("root input %+v != translate input %+v", h1.Translator.lastInput, h2.Translator.lastInput)
	}
}

func TestContextFlag(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--context", "Did you finish the report?", "--text", "Да, закончил вчера"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.Translator.lastInput.Context != "Did you finish the report?" {
		t.Fatalf("Context = %q", h.Translator.lastInput.Context)
	}
	if h.Translator.lastInput.Source != "Да, закончил вчера" {
		t.Fatalf("Source = %q", h.Translator.lastInput.Source)
	}
}

func TestPipedStdin(t *testing.T) {
	h := newHarness(t)
	h.Stdin.WriteString("piped input\n")
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.Translator.lastInput.Source != "piped input\n" {
		t.Fatalf("Source = %q", h.Translator.lastInput.Source)
	}
	if h.Translator.calls != 1 {
		t.Fatalf("Translate called %d times, want 1", h.Translator.calls)
	}
}

func TestPipedStdinViaTranslateSubcommand(t *testing.T) {
	h := newHarness(t)
	h.Stdin.WriteString("piped input\n")
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"translate"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.Translator.lastInput.Source != "piped input\n" {
		t.Fatalf("Source = %q", h.Translator.lastInput.Source)
	}
}

func TestTextFlagPreventsStdinRead(t *testing.T) {
	h := newHarness(t)
	h.Stdin.WriteString("should not be read")
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--text", "hello"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.Translator.lastInput.Source != "hello" {
		t.Fatalf("Source = %q, want %q (stdin must not be read when --text is set)", h.Translator.lastInput.Source, "hello")
	}
	if h.Stdin.Len() == 0 {
		t.Fatal("stdin was consumed even though --text was set")
	}
}

func TestTerminalStdinAndStdoutLaunchesTUI(t *testing.T) {
	h := newHarness(t)
	h.StdinTerminal = true
	h.StdoutTerminal = true
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.TUICalls != 1 {
		t.Fatalf("TUI runner called %d times, want 1", h.TUICalls)
	}
	if h.Translator.calls != 0 {
		t.Fatalf("Translate called %d times, want 0 (TUI owns translation)", h.Translator.calls)
	}
	if h.TUIModel != "test-model" {
		t.Fatalf("TUIModel = %q, want %q", h.TUIModel, "test-model")
	}
}

func TestTerminalStdinNonTerminalStdoutIsError(t *testing.T) {
	h := newHarness(t)
	h.StdinTerminal = true
	h.StdoutTerminal = false
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("ExitCode = %d, want 1", ExitCode(err))
	}
	if h.TUICalls != 0 {
		t.Fatalf("TUI runner called %d times, want 0", h.TUICalls)
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0 (no network access)", h.TranslatorCalls)
	}
	if !strings.Contains(h.Stderr.String(), "interactive mode requires terminal output") {
		t.Fatalf("stderr = %q, want it to mention the terminal-output requirement", h.Stderr.String())
	}
}

func TestEmptyTextFlagIsError(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--text", ""})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error for empty --text")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("ExitCode = %d, want 1", ExitCode(err))
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0 (no network access for invalid input)", h.TranslatorCalls)
	}
}

func TestWhitespaceOnlyTextFlagIsError(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--text", "   "})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error for whitespace-only --text")
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
	}
}

func TestWhitespaceOnlyStdinIsError(t *testing.T) {
	h := newHarness(t)
	h.Stdin.WriteString("   \n\t  ")
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error for whitespace-only stdin")
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
	}
}

func TestPositionalTextIsUsageError(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"Текст"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want usage error for positional text")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("ExitCode = %d, want 2 (usage error)", ExitCode(err))
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--bogus-flag"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want usage error for unknown flag")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("ExitCode = %d, want 2 (usage error)", ExitCode(err))
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"bogus-command"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want usage error for unknown command")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("ExitCode = %d, want 2 (usage error)", ExitCode(err))
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
	}
}

func TestConfigurationErrorMakesNoHTTPRequest(t *testing.T) {
	h := newHarness(t)
	h.Env = map[string]string{} // no config file and no env -> invalid config
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--text", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want configuration error")
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
	}
	if h.Translator.calls != 0 {
		t.Fatalf("Translate called %d times, want 0", h.Translator.calls)
	}
}

func TestStdoutAndStderrRemainSeparate(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--text", "hello"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if strings.Contains(h.Stdout.String(), "qt:") {
		t.Fatalf("stdout contains status output: %q", h.Stdout.String())
	}
	if strings.Contains(h.Stderr.String(), "translated") {
		t.Fatalf("stderr contains translation output: %q", h.Stderr.String())
	}
	if h.Stdout.String() != "translated\n" {
		t.Fatalf("stdout = %q, want exactly %q", h.Stdout.String(), "translated\n")
	}
}

func TestTranslationFailurePropagatesToStderrAndExitCode(t *testing.T) {
	h := newHarness(t)
	h.Translator.err = &fakeSafeError{msg: "request timed out"}
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--text", "hello"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("ExitCode = %d, want 1", ExitCode(err))
	}
	if h.Stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", h.Stdout.String())
	}
}

type fakeSafeError struct{ msg string }

func (e *fakeSafeError) Error() string { return e.msg }

func TestProfileFlagSelectsProfile(t *testing.T) {
	h := newHarness(t)
	h.Env = map[string]string{} // no field/profile env overrides
	store := config.Store{
		Active: "litellm",
		Profiles: map[string]config.Config{
			"litellm": {BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "lk"},
			"nvidia":  {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "llama", APIKey: "nv"},
		},
	}
	if err := config.SaveStore(h.ConfigPath, store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--profile", "nvidia", "--text", "hello"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	want := store.Profiles["nvidia"]
	if h.LastTranslatorConfig != want {
		t.Fatalf("translator built with %+v, want the nvidia profile %+v", h.LastTranslatorConfig, want)
	}
}

func TestActiveProfileUsedByDefault(t *testing.T) {
	h := newHarness(t)
	h.Env = map[string]string{}
	store := config.Store{
		Active: "litellm",
		Profiles: map[string]config.Config{
			"litellm": {BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "lk"},
			"nvidia":  {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "llama", APIKey: "nv"},
		},
	}
	if err := config.SaveStore(h.ConfigPath, store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--text", "hello"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if h.LastTranslatorConfig != store.Profiles["litellm"] {
		t.Fatalf("translator built with %+v, want the active litellm profile", h.LastTranslatorConfig)
	}
}

func TestProfileFlagPassedToTUI(t *testing.T) {
	h := newHarness(t)
	h.StdinTerminal = true
	h.StdoutTerminal = true
	store := config.Store{
		Active: "litellm",
		Profiles: map[string]config.Config{
			"litellm": {BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "lk"},
			"nvidia":  {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "llama", APIKey: "nv"},
		},
	}
	if err := config.SaveStore(h.ConfigPath, store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}
	h.Env = map[string]string{}

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"--profile", "nvidia"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.TUICalls != 1 {
		t.Fatalf("TUI called %d times, want 1", h.TUICalls)
	}
	if h.TUIProfile != "nvidia" {
		t.Fatalf("TUI profile override = %q, want nvidia", h.TUIProfile)
	}
	if h.TUIModel != "llama" {
		t.Fatalf("TUI model = %q, want llama", h.TUIModel)
	}
}
