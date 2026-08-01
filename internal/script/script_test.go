package script

import (
	"strings"
	"testing"
)

// The whole point of the package is that timing survives untouched, so most of
// these assert on what is *not* spoken as much as on what is.

func TestParseSplitsTimingFromSpeech(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantPrefix string
		wantText   string
	}{
		{"plain line", "Привет!", "", "Привет!"},
		{"clock timestamp", "[1:20] Привет!", "[1:20] ", "Привет!"},
		{"hour timestamp", "[0:01:20] Привет!", "[0:01:20] ", "Привет!"},
		{"fractional timestamp", "[1:20.5] Привет!", "[1:20.5] ", "Привет!"},
		{"bare seconds", "[12] Привет!", "[12] ", "Привет!"},
		{"relative", "[+2.5] Привет!", "[+2.5] ", "Привет!"},
		{"delay with a space", "$delay 2s Привет!", "$delay 2s ", "Привет!"},
		{"delay in parentheses", "$delay(0.5s) Привет!", "$delay(0.5s) ", "Привет!"},
		{"delay without the dollar", "delay(10s) Привет!", "delay(10s) ", "Привет!"},
		{"stacked delays", "$delay 1s $delay 500ms Привет!", "$delay 1s $delay 500ms ", "Привет!"},
		{"delay then timestamp", "$delay 1s [1:20] Привет!", "$delay 1s [1:20] ", "Привет!"},
		{"indented", "  [1:20] Привет!", "  [1:20] ", "Привет!"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line := Parse(test.raw).Lines[0]
			if line.Prefix != test.wantPrefix {
				t.Errorf("prefix = %q, want %q", line.Prefix, test.wantPrefix)
			}
			if line.Text != test.wantText {
				t.Errorf("text = %q, want %q", line.Text, test.wantText)
			}
			if got := line.Prefix + line.Text; got != test.raw {
				t.Errorf("prefix+text = %q, does not reconstruct %q", got, test.raw)
			}
		})
	}
}

func TestParseLeavesStructureUnspoken(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"blank", ""},
		{"whitespace", "   "},
		{"comment", "# Транскрипция"},
		{"indented comment", "  # Транскрипция"},
		{"voice directive", "@voice am_eric"},
		{"speed directive", "@speed 0.9"},
		{"duration directive", "@duration 19:45"},
		{"bare delay", "$delay 10s"},
		{"bare delay in parentheses", "delay(10s)"},
		{"timestamp with nothing after it", "[1:20]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Surrounded by other lines, so that a blank one is a line in the
			// middle of a script rather than the trailing newline of a file.
			line := Parse("@voice am_eric\n" + test.raw + "\n@speed 1.0").Lines[1]
			if line.Spoken() {
				t.Errorf("line %q was treated as speech (text %q)", test.raw, line.Text)
			}
			if line.Render("TRANSLATED") != test.raw {
				t.Errorf("rendering changed %q to %q", test.raw, line.Render("TRANSLATED"))
			}
		})
	}
}

func TestParseTreatsAnUnrecognisedLineAsSpeech(t *testing.T) {
	// A malformed timestamp is speech here, deliberately: dropping a line from
	// the soundtrack is worse than translating something that did not need it.
	line := Parse("[1:2:3 Привет!").Lines[0]
	if !line.Spoken() {
		t.Fatalf("line was not treated as speech")
	}
	if line.Text != "[1:2:3 Привет!" {
		t.Errorf("text = %q", line.Text)
	}
}

const sample = `# Транскрипция
@voice am_eric
@speed 0.9

[0:02] Привет! Сегодня я покажу новые возможности.
$delay(0.5s) Первая из них - пресеты.
Вот как они выглядят.
[1:20] Дальше - снапшоты.
$delay 10s
Готово.
@duration 1:45
`

func TestRenderIntoTouchesOnlySpeech(t *testing.T) {
	parsed := Parse(sample)
	if got := parsed.SpokenCount(); got != 5 {
		t.Fatalf("spoken lines = %d, want 5", got)
	}

	translations := make(map[int]string)
	for _, index := range parsed.SpokenIndexes() {
		translations[index] = "ENGLISH"
	}
	rendered := parsed.RenderInto(translations)

	for _, structure := range []string{
		"# Транскрипция", "@voice am_eric", "@speed 0.9", "@duration 1:45",
		"[0:02] ENGLISH", "$delay(0.5s) ENGLISH", "[1:20] ENGLISH", "$delay 10s",
	} {
		if !strings.Contains(rendered, structure) {
			t.Errorf("rendered script lost %q:\n%s", structure, rendered)
		}
	}
	if strings.Contains(rendered, "Привет") || strings.Contains(rendered, "снапшоты") {
		t.Errorf("rendered script still holds Russian speech:\n%s", rendered)
	}
	if lines := strings.Count(rendered, "\n"); lines != strings.Count(sample, "\n") {
		t.Errorf("line count changed: %d, want %d", lines, strings.Count(sample, "\n"))
	}
}

func TestRenderIntoKeepsTheSourceForMissingTranslations(t *testing.T) {
	parsed := Parse("Привет!\nПока!")
	rendered := parsed.RenderInto(map[int]string{0: "Hello!"})
	if rendered != "Hello!\nПока!\n" {
		t.Errorf("rendered = %q", rendered)
	}
}

func TestParseAndRenderRoundTripAnUntranslatedScript(t *testing.T) {
	if got := Parse(sample).RenderInto(nil); got != sample {
		t.Errorf("round trip changed the script:\n%q\nwant:\n%q", got, sample)
	}
}

func TestParseHandlesAnEmptyScript(t *testing.T) {
	parsed := Parse("")
	if len(parsed.Lines) != 0 || parsed.SpokenCount() != 0 {
		t.Errorf("empty script parsed to %d line(s)", len(parsed.Lines))
	}
}
