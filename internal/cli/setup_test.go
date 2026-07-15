package cli

import (
	"strings"
	"testing"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/config"
)

func TestSetupInteractiveUsesFormRunner(t *testing.T) {
	h := newHarness(t)
	h.StdinTerminal = true
	h.StdoutTerminal = true
	h.SetupFormResult = app.SetupResult{
		Profile:     "litellm",
		Config:      config.Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "secret"},
		Translation: "Hello!",
	}

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.SetupFormCalls != 1 {
		t.Fatalf("setup form called %d times, want 1", h.SetupFormCalls)
	}
	if h.Translator.calls != 0 {
		t.Fatalf("Translate called %d times, want 0 (setup must not invoke normal translation)", h.Translator.calls)
	}
	if h.Stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", h.Stdout.String())
	}
	if !strings.Contains(h.Stderr.String(), "Hello!") {
		t.Fatalf("stderr = %q, want it to mention the test translation", h.Stderr.String())
	}
}

func TestSetupInteractivePassesStore(t *testing.T) {
	h := newHarness(t)
	h.StdinTerminal = true
	h.StdoutTerminal = true
	store := config.Store{
		Active:   "nvidia",
		Profiles: map[string]config.Config{"nvidia": {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "llama", APIKey: "nv"}},
	}
	if err := config.SaveStore(h.ConfigPath, store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.SetupFormStore.Active != "nvidia" {
		t.Fatalf("form store active = %q, want nvidia", h.SetupFormStore.Active)
	}
	if _, ok := h.SetupFormStore.Profiles["nvidia"]; !ok {
		t.Fatal("form store did not include the existing nvidia profile")
	}
}

func TestSetupInteractiveWithoutTerminalIsError(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error without a terminal")
	}
	if h.SetupFormCalls != 0 {
		t.Fatalf("setup form called %d times, want 0", h.SetupFormCalls)
	}
}

func TestSetupAddWithFlags(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup", "add", "litellm", "--base-url", "http://localhost:4000/v1", "--model", "translategemma", "--api-key", "secret"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.SetupFormCalls != 0 {
		t.Fatalf("setup form called %d times, want 0 (subcommand bypasses the interactive form)", h.SetupFormCalls)
	}
	if h.Translator.calls != 1 {
		t.Fatalf("Translate called %d times, want 1 (endpoint test)", h.Translator.calls)
	}
	if h.Translator.lastInput.Source != "Привет!" {
		t.Fatalf("Source = %q, want the fixed setup test source", h.Translator.lastInput.Source)
	}
	if h.Stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", h.Stdout.String())
	}

	store, err := config.LoadStore(h.ConfigPath)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if store.Active != "litellm" {
		t.Fatalf("active = %q, want litellm", store.Active)
	}
	want := config.Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "secret"}
	if store.Profiles["litellm"] != want {
		t.Fatalf("saved profile = %+v, want %+v", store.Profiles["litellm"], want)
	}
}

func TestSetupAddMissingFlagIsError(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup", "add", "litellm", "--base-url", "http://localhost:4000/v1"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error when model/api-key are missing")
	}
	if h.Translator.calls != 0 {
		t.Fatalf("Translate called %d times, want 0", h.Translator.calls)
	}
}

func TestSetupAddRequiresName(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup", "add", "--base-url", "http://localhost:4000/v1", "--model", "m", "--api-key", "k"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error when the profile name is missing")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("ExitCode = %d, want 2 (usage error for missing arg)", ExitCode(err))
	}
}

func TestSetupAddFailedTestDoesNotSave(t *testing.T) {
	h := newHarness(t)
	h.Translator.err = &fakeSafeError{msg: "boom"}
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup", "add", "litellm", "--base-url", "http://localhost:4000/v1", "--model", "m", "--api-key", "k"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil error, want error")
	}
	if ExitCode(err) != 1 {
		t.Fatalf("ExitCode = %d, want 1", ExitCode(err))
	}
	store, _ := config.LoadStore(h.ConfigPath)
	if len(store.Profiles) != 0 {
		t.Fatalf("profiles must not be written on a failed test: %+v", store.Profiles)
	}
	if got := strings.Count(h.Stderr.String(), "boom"); got != 1 {
		t.Fatalf("stderr mentions the error %d times, want exactly once: %q", got, h.Stderr.String())
	}
}

func TestSetupList(t *testing.T) {
	h := newHarness(t)
	store := config.Store{
		Active: "litellm",
		Profiles: map[string]config.Config{
			"litellm": {BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "SECRETKEYONE"},
			"nvidia":  {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "llama", APIKey: "SECRETKEYTWO"},
		},
	}
	if err := config.SaveStore(h.ConfigPath, store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	out := h.Stdout.String()
	if !strings.Contains(out, "litellm") || !strings.Contains(out, "nvidia") {
		t.Fatalf("list output missing profiles: %q", out)
	}
	// The active profile is marked and no API key is shown.
	if !strings.Contains(out, "* litellm") {
		t.Fatalf("active profile not marked: %q", out)
	}
	if strings.Contains(out, "SECRETKEY") {
		t.Fatalf("list output must not contain API keys: %q", out)
	}
}

func TestSetupUse(t *testing.T) {
	h := newHarness(t)
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
	root.SetArgs([]string{"setup", "use", "nvidia"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	got, _ := config.LoadStore(h.ConfigPath)
	if got.Active != "nvidia" {
		t.Fatalf("active = %q, want nvidia", got.Active)
	}
	if h.Translator.calls != 0 {
		t.Fatalf("use must not test the endpoint, got %d calls", h.Translator.calls)
	}
}

func TestSetupUseUnknownIsError(t *testing.T) {
	h := newHarness(t)
	store := config.Store{Active: "litellm", Profiles: map[string]config.Config{"litellm": {BaseURL: "http://localhost:4000/v1", Model: "m", APIKey: "k"}}}
	if err := config.SaveStore(h.ConfigPath, store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup", "use", "ghost"})
	if err := root.Execute(); err == nil {
		t.Fatal("Execute() = nil error, want error for unknown profile")
	}
}

func TestSetupRemove(t *testing.T) {
	h := newHarness(t)
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
	root.SetArgs([]string{"setup", "remove", "nvidia"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	got, _ := config.LoadStore(h.ConfigPath)
	if _, ok := got.Profiles["nvidia"]; ok {
		t.Fatal("nvidia profile was not removed")
	}
}

func TestSetupRemoveActiveIsError(t *testing.T) {
	h := newHarness(t)
	store := config.Store{
		Active: "litellm",
		Profiles: map[string]config.Config{
			"litellm": {BaseURL: "http://localhost:4000/v1", Model: "m", APIKey: "k"},
			"nvidia":  {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "l", APIKey: "n"},
		},
	}
	if err := config.SaveStore(h.ConfigPath, store); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"setup", "remove", "litellm"})
	if err := root.Execute(); err == nil {
		t.Fatal("Execute() = nil error, want error removing the active profile")
	}

	got, _ := config.LoadStore(h.ConfigPath)
	if _, ok := got.Profiles["litellm"]; !ok {
		t.Fatal("active profile must not be removed")
	}
}
