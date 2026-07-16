package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/translate"
)

// fakeTranslator records the input it received and returns a canned result.
type fakeTranslator struct {
	response  string
	err       error
	lastInput translate.TranslationInput
	calls     int
}

func (f *fakeTranslator) Translate(_ context.Context, input translate.TranslationInput, reporter translate.Reporter) (string, error) {
	f.calls++
	f.lastInput = input
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func TestTestAndSaveUsesProductionPromptAndTranslator(t *testing.T) {
	fake := &fakeTranslator{response: "Hello!"}
	var gotConfig config.Config
	newTranslator := func(cfg config.Config) (translate.Translator, error) {
		gotConfig = cfg
		return fake, nil
	}

	path := filepath.Join(t.TempDir(), "config.json")
	candidate := config.Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "secret"}

	result, err := TestAndSave(context.Background(), path, "litellm", candidate, newTranslator, translate.DiscardReporter{})
	if err != nil {
		t.Fatalf("TestAndSave() error: %v", err)
	}
	if result.Translation != "Hello!" {
		t.Fatalf("Translation = %q, want %q", result.Translation, "Hello!")
	}
	if result.Name != "litellm" {
		t.Fatalf("Name = %q, want litellm", result.Name)
	}
	if gotConfig != candidate {
		t.Fatalf("newTranslator called with %+v, want %+v", gotConfig, candidate)
	}
	if fake.calls != 1 {
		t.Fatalf("Translate called %d times, want 1", fake.calls)
	}
	if fake.lastInput.Source != testSource {
		t.Fatalf("Source = %q, want %q", fake.lastInput.Source, testSource)
	}
	if fake.lastInput.Context != "" {
		t.Fatalf("Context = %q, want empty", fake.lastInput.Context)
	}
}

func TestTestAndSaveSuccessSavesProfileAndActivates(t *testing.T) {
	fake := &fakeTranslator{response: "Hello!"}
	newTranslator := func(config.Config) (translate.Translator, error) { return fake, nil }

	path := filepath.Join(t.TempDir(), "config.json")
	candidate := config.Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "secret"}

	if _, err := TestAndSave(context.Background(), path, "litellm", candidate, newTranslator, translate.DiscardReporter{}); err != nil {
		t.Fatalf("TestAndSave() error: %v", err)
	}

	store, err := config.LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if store.Active != "litellm" {
		t.Fatalf("active = %q, want litellm", store.Active)
	}
	if store.Profiles["litellm"] != candidate {
		t.Fatalf("saved profile = %+v, want %+v", store.Profiles["litellm"], candidate)
	}
}

func TestTestAndSavePreservesOtherProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	existing := config.Store{
		Active:   "nvidia",
		Profiles: map[string]config.Config{"nvidia": {BaseURL: "https://integrate.api.nvidia.com/v1", Model: "llama", APIKey: "nv"}},
	}
	if err := config.SaveStore(path, existing); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	fake := &fakeTranslator{response: "Hello!"}
	newTranslator := func(config.Config) (translate.Translator, error) { return fake, nil }
	candidate := config.Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "lk"}

	if _, err := TestAndSave(context.Background(), path, "litellm", candidate, newTranslator, translate.DiscardReporter{}); err != nil {
		t.Fatalf("TestAndSave() error: %v", err)
	}

	store, err := config.LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if _, ok := store.Profiles["nvidia"]; !ok {
		t.Fatal("existing nvidia profile was lost")
	}
	if _, ok := store.Profiles["litellm"]; !ok {
		t.Fatal("new litellm profile was not added")
	}
	if store.Active != "litellm" {
		t.Fatalf("active = %q, want litellm (the just-configured profile)", store.Active)
	}
}

func TestTestAndSaveFailedTestDoesNotCreateConfig(t *testing.T) {
	fake := &fakeTranslator{err: errors.New("boom")}
	newTranslator := func(config.Config) (translate.Translator, error) { return fake, nil }

	path := filepath.Join(t.TempDir(), "config.json")
	candidate := config.Config{BaseURL: "http://localhost:4000/v1", Model: "translategemma", APIKey: "secret"}

	if _, err := TestAndSave(context.Background(), path, "litellm", candidate, newTranslator, translate.DiscardReporter{}); err == nil {
		t.Fatal("TestAndSave() = nil error, want error")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file must not be created after a failed test, Stat() error = %v", err)
	}
}

func TestTestAndSaveFailedTestDoesNotOverwriteExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := config.Store{
		Active:   "default",
		Profiles: map[string]config.Config{"default": {BaseURL: "http://original:1/v1", Model: "original-model", APIKey: "original-key"}},
	}
	if err := config.SaveStore(path, original); err != nil {
		t.Fatalf("SaveStore() error: %v", err)
	}

	fake := &fakeTranslator{err: errors.New("boom")}
	newTranslator := func(config.Config) (translate.Translator, error) { return fake, nil }
	broken := config.Config{BaseURL: "http://broken:1/v1", Model: "broken-model", APIKey: "broken-key"}

	if _, err := TestAndSave(context.Background(), path, "default", broken, newTranslator, translate.DiscardReporter{}); err == nil {
		t.Fatal("TestAndSave() = nil error, want error")
	}

	store, err := config.LoadStore(path)
	if err != nil {
		t.Fatalf("LoadStore() error: %v", err)
	}
	if store.Profiles["default"] != original.Profiles["default"] {
		t.Fatalf("profile changed after a failed test: %+v", store.Profiles["default"])
	}
}

func TestTestAndSaveInvalidCandidateNeverCallsTranslator(t *testing.T) {
	fake := &fakeTranslator{response: "Hello!"}
	called := false
	newTranslator := func(config.Config) (translate.Translator, error) {
		called = true
		return fake, nil
	}

	path := filepath.Join(t.TempDir(), "config.json")
	invalid := config.Config{BaseURL: "not a url", Model: "m", APIKey: "k"}

	if _, err := TestAndSave(context.Background(), path, "x", invalid, newTranslator, translate.DiscardReporter{}); err == nil {
		t.Fatal("TestAndSave() = nil error, want error for invalid candidate")
	}
	if called {
		t.Fatal("newTranslator must not be called for an invalid candidate")
	}
}
