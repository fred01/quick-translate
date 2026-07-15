package cli

import "testing"

func TestVersionCommand(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if h.TranslatorCalls != 0 {
		t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
	}
	if h.Stdout.String() == "" {
		t.Fatal("version command produced no stdout")
	}
}

func TestCompletionCommand(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			h := newHarness(t)
			root := NewRootCommand(h.Deps())
			root.SetArgs([]string{"completion", shell})

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			if h.Stdout.Len() == 0 {
				t.Fatalf("completion %s produced no output", shell)
			}
			if h.TranslatorCalls != 0 {
				t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
			}
		})
	}
}

func TestHelpMakesNoHTTPRequest(t *testing.T) {
	for _, args := range [][]string{
		{"--help"},
		{"translate", "--help"},
		{"setup", "--help"},
	} {
		t.Run(args[0], func(t *testing.T) {
			h := newHarness(t)
			root := NewRootCommand(h.Deps())
			root.SetArgs(args)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			if h.TranslatorCalls != 0 {
				t.Fatalf("NewTranslator called %d times, want 0", h.TranslatorCalls)
			}
			if h.SetupFormCalls != 0 {
				t.Fatalf("setup form called %d times, want 0", h.SetupFormCalls)
			}
			if h.Stdout.String() == "" {
				t.Fatal("--help produced no output")
			}
		})
	}
}
