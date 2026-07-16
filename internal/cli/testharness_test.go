package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/fred01/quick-translate/internal/app"
	"github.com/fred01/quick-translate/internal/config"
	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

// fakeTranslator stands in for a real network call so CLI tests never make
// one; it records exactly what it was asked to translate.
type fakeTranslator struct {
	response  string
	err       error
	calls     int
	lastInput translate.TranslationInput
}

func (f *fakeTranslator) Translate(_ context.Context, input translate.TranslationInput, reporter translate.Reporter) (string, error) {
	f.calls++
	f.lastInput = input
	if f.err != nil {
		reporter.Report(translate.Status{Stage: translate.StageFailed, Err: f.err})
		return "", f.err
	}
	reporter.Report(translate.Status{Stage: translate.StageCompleted, OutputChars: len(f.response)})
	return f.response, nil
}

func (f *fakeTranslator) TranslateMulti(_ context.Context, _, _ string, tones []prompt.Tone, _ translate.Reporter) (map[prompt.Tone]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[prompt.Tone]string, len(tones))
	for _, t := range tones {
		out[t] = f.response
	}
	return out, nil
}

// harness builds a fresh, fully faked app.Dependencies for one test, and
// records every call made through it so tests can assert, in particular,
// that invalid CLI input never reaches the network.
type harness struct {
	Stdin  *bytes.Buffer
	Stdout *bytes.Buffer
	Stderr *bytes.Buffer

	StdinTerminal  bool
	StdoutTerminal bool

	Translator           *fakeTranslator
	NewTranslatorErr     error
	TranslatorCalls      int
	LastTranslatorConfig config.Config

	TUICalls   int
	TUIErr     error
	TUIModel   string
	TUIHost    string
	TUIProfile string
	TUITone    prompt.Tone

	SetupFormCalls  int
	SetupFormErr    error
	SetupFormResult app.SetupResult
	SetupFormStore  config.Store

	ConfigPath string
	Env        map[string]string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return &harness{
		Stdin:      &bytes.Buffer{},
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		Translator: &fakeTranslator{response: "translated"},
		ConfigPath: filepath.Join(t.TempDir(), "config.json"),
		Env: map[string]string{
			config.EnvBaseURL: "http://localhost:4000/v1",
			config.EnvModel:   "test-model",
			config.EnvAPIKey:  "test-key",
		},
	}
}

func (h *harness) Deps() app.Dependencies {
	return app.Dependencies{
		Stdin:  h.Stdin,
		Stdout: h.Stdout,
		Stderr: h.Stderr,

		ConfigPath: h.ConfigPath,
		Getenv:     func(k string) string { return h.Env[k] },

		StdinIsTerminal:  func() bool { return h.StdinTerminal },
		StdoutIsTerminal: func() bool { return h.StdoutTerminal },

		NewTranslator: func(cfg config.Config) (translate.Translator, error) {
			h.TranslatorCalls++
			h.LastTranslatorConfig = cfg
			if h.NewTranslatorErr != nil {
				return nil, h.NewTranslatorErr
			}
			return h.Translator, nil
		},

		RunTUI: func(in io.Reader, out io.Writer, configPath string, getenv config.EnvLookup, newTranslator func(config.Config) (translate.Translator, error), profile string, tone prompt.Tone) error {
			h.TUICalls++
			h.TUIProfile = profile
			h.TUITone = tone
			if cfg, _, err := config.Effective(configPath, getenv, profile); err == nil {
				h.TUIModel = cfg.Model
				h.TUIHost = translate.SafeHost(cfg.BaseURL)
			}
			return h.TUIErr
		},

		RunSetupForm: func(in io.Reader, out io.Writer, configPath string, store config.Store, newTranslator func(config.Config) (translate.Translator, error)) (app.SetupResult, error) {
			h.SetupFormCalls++
			h.SetupFormStore = store
			return h.SetupFormResult, h.SetupFormErr
		},
	}
}
