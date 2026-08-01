package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fred01/quick-translate/internal/prompt"
	"github.com/fred01/quick-translate/internal/translate"
)

const sampleScript = `# Транскрипция
@voice am_eric
@speed 0.9

[0:02] Привет!
$delay(0.5s) Первая возможность - пресеты.
[1:20] Дальше снапшоты.
@duration 1:45
`

// numberedEcho answers a numbered request with the same numbering, so the CLI
// tests exercise the real reassembly rather than a canned string.
func numberedEcho(source string) string {
	var out []string
	for i, line := range strings.Split(source, "\n") {
		_, text, found := strings.Cut(line, ". ")
		if !found {
			continue
		}
		out = append(out, strings.Fields(line)[0]+" EN"+string(rune('A'+i))+": "+text)
	}
	return strings.Join(out, "\n")
}

func writeScriptFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "demo.script")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestScriptTranslatesAFileAndKeepsItsTiming(t *testing.T) {
	h := newHarness(t)
	path := writeScriptFile(t, sampleScript)
	h.Translator.respondTo = numberedEcho

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"script", path})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr: %s)", err, h.Stderr)
	}

	out := h.Stdout.String()
	for _, structure := range []string{"# Транскрипция", "@voice am_eric", "@speed 0.9", "@duration 1:45"} {
		if !strings.Contains(out, structure) {
			t.Errorf("output lost %q:\n%s", structure, out)
		}
	}
	for _, prefix := range []string{"[0:02] EN", "$delay(0.5s) EN", "[1:20] EN"} {
		if !strings.Contains(out, prefix) {
			t.Errorf("output lost the timing prefix %q:\n%s", prefix, out)
		}
	}
	if strings.Count(out, "\n") != strings.Count(sampleScript, "\n") {
		t.Errorf("line count changed:\n%s", out)
	}
}

func TestScriptNeverSendsTimingToTheModel(t *testing.T) {
	h := newHarness(t)
	h.Translator.respondTo = numberedEcho

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"script", writeScriptFile(t, sampleScript)})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	sent := h.Translator.lastInput.Source
	for _, timing := range []string{"[0:02]", "[1:20]", "$delay", "@voice", "@duration", "Транскрипция"} {
		if strings.Contains(sent, timing) {
			t.Errorf("the request carried %q:\n%s", timing, sent)
		}
	}
	if h.Translator.lastInput.Kind != translate.PromptScript {
		t.Errorf("prompt kind = %v, want PromptScript", h.Translator.lastInput.Kind)
	}
}

func TestScriptWritesToTheOutputFile(t *testing.T) {
	h := newHarness(t)
	h.Translator.respondTo = numberedEcho
	target := filepath.Join(t.TempDir(), "demo.en.script")

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"script", writeScriptFile(t, sampleScript), "-o", target})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(written), "[0:02] EN") {
		t.Errorf("output file:\n%s", written)
	}
	if !strings.Contains(h.Stdout.String(), target) {
		t.Errorf("stdout did not name the file it wrote: %q", h.Stdout)
	}
}

func TestScriptReadsStdin(t *testing.T) {
	h := newHarness(t)
	h.Translator.respondTo = numberedEcho
	h.Stdin.WriteString(sampleScript)

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"script"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(h.Stdout.String(), "[1:20] EN") {
		t.Errorf("stdout:\n%s", h.Stdout)
	}
}

func TestScriptAcceptsATone(t *testing.T) {
	h := newHarness(t)
	h.Translator.respondTo = numberedEcho

	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"script", writeScriptFile(t, sampleScript), "--tone", "literal"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if h.Translator.lastInput.Tone != prompt.ToneLiteral {
		t.Errorf("tone = %v, want literal", h.Translator.lastInput.Tone)
	}
}

func TestScriptRejectsAnUnknownToneBeforeReachingTheNetwork(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"script", writeScriptFile(t, sampleScript), "--tone", "shouty"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error")
	}
	if h.Translator.calls != 0 {
		t.Errorf("made %d request(s) despite a bad tone", h.Translator.calls)
	}
}

func TestScriptRefusesAScriptWithNothingToSay(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"script", writeScriptFile(t, "# a note\n@voice am_eric\n$delay 10s\n")})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error")
	}
	if h.Translator.calls != 0 {
		t.Errorf("made %d request(s) for a script with no speech", h.Translator.calls)
	}
	if !strings.Contains(h.Stderr.String(), "no spoken lines") {
		t.Errorf("stderr: %q", h.Stderr)
	}
}

func TestScriptReportsAMissingFile(t *testing.T) {
	h := newHarness(t)
	root := NewRootCommand(h.Deps())
	root.SetArgs([]string{"script", filepath.Join(t.TempDir(), "absent.script")})
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(h.Stderr.String(), "cannot read") {
		t.Errorf("stderr: %q", h.Stderr)
	}
}
